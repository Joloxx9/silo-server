package requests

import (
	"context"
	"errors"
	"testing"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// activeRequestFor seeds an active request for the title, owned by another
// account's profile.
func activeRequestFor(store *fakeStore, tmdbID int) *Request {
	req := &Request{
		ID: "req-owner", MediaType: MediaTypeMovie, TMDBID: tmdbID, Title: "Heat",
		Status: StatusPending, Outcome: OutcomeActive,
		RequestedByUserID: 2, RequestedByProfileID: "owner-profile",
	}
	store.requests[req.ID] = req
	store.active[MediaTypeMovie][tmdbID] = req
	return req
}

func TestFollowTitleSomeoneElseRequested(t *testing.T) {
	store := newFakeStore()
	activeRequestFor(store, 949)
	svc := newTestService(store)

	state, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949)
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if !state.Following || state.RequestedByViewer || state.Requestable || state.Reason != "already_requested" || state.RequestID != "" {
		t.Fatalf("state = %+v, want following, not requestable, request id hidden from another account", state)
	}
	followed, _ := store.FollowedTitles(context.Background(), MediaTypeMovie, []int{949}, "profile-1")
	if !followed[949] {
		t.Fatal("follow was not stored")
	}
	if _, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); err != nil {
		t.Fatalf("second Follow: %v (want idempotent)", err)
	}

	if err := svc.Unfollow(context.Background(), testViewer(1), MediaTypeMovie, 949); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	followed, _ = store.FollowedTitles(context.Background(), MediaTypeMovie, []int{949}, "profile-1")
	if followed[949] {
		t.Fatal("follow survived Unfollow")
	}
}

func TestFollowOwnRequestStoresNothing(t *testing.T) {
	store := newFakeStore()
	req := activeRequestFor(store, 949)
	req.RequestedByUserID, req.RequestedByProfileID = 1, "profile-1"
	svc := newTestService(store)

	state, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949)
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if !state.Following || !state.RequestedByViewer || state.RequestID != "req-owner" {
		t.Fatalf("state = %+v, want following, requested by the viewer, with its request id", state)
	}
	if len(store.follows) != 0 {
		t.Fatalf("follows = %v, want none: the requester is always notified", store.follows)
	}
}

func TestFollowRefusesTitleWithoutActiveRequest(t *testing.T) {
	svc := newTestService(newFakeStore())
	if _, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); !errors.Is(err, ErrNotRequested) {
		t.Fatalf("err = %v, want ErrNotRequested", err)
	}
}

// A title's open request is what makes it followable: a series partly in the
// library can have one for its missing seasons, and a title in the library
// with no open request has nothing to follow.
func TestFollowNeedsAnOpenRequestNotAnEmptyLibrary(t *testing.T) {
	store := newFakeStore()
	activeRequestFor(store, 949)
	svc := NewService(store, &fakeTMDBClient{}, presentMovie(949))
	svc.SetUserRepository(requestUserRepo{})
	if _, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); err != nil {
		t.Fatalf("follow an open request for a title partly in the library: %v", err)
	}
	if _, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 950); !errors.Is(err, ErrNotRequested) {
		t.Fatalf("follow a title with no open request: err = %v, want ErrNotRequested", err)
	}
}

func TestFollowRefusesWhenRequestsDisabled(t *testing.T) {
	store := newFakeStore()
	store.settings.RequestsEnabled = false
	activeRequestFor(store, 949)
	if _, err := newTestService(store).Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); !errors.Is(err, ErrRequestsDisabled) {
		t.Fatalf("err = %v, want ErrRequestsDisabled", err)
	}
}

func TestFollowRefusesBlockedAccount(t *testing.T) {
	store := newFakeStore()
	activeRequestFor(store, 949)
	store.limit = &UserLimit{UserID: 1, LimitMode: LimitModeBlocked, ApprovalMode: ApprovalModeInherit}
	if _, err := newTestService(store).Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("err = %v, want ErrUserBlocked", err)
	}
}

// A declined or withdrawn request is no longer on its way, so its title's
// follows are dropped rather than left where the follower cannot see them.
func TestWithdrawingRequestForgetsFollows(t *testing.T) {
	for _, tc := range []struct {
		name     string
		withdraw func(*Service) error
	}{
		{"decline", func(s *Service) error {
			_, err := s.Decline(context.Background(), Viewer{UserID: 9, IsAdmin: true}, "req-owner", "")
			return err
		}},
		{"cancel", func(s *Service) error {
			_, err := s.Cancel(context.Background(), Viewer{UserID: 2, ProfileID: "owner-profile"}, "req-owner", "")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			activeRequestFor(store, 949)
			svc := newTestService(store)
			if _, err := svc.Follow(context.Background(), testViewer(1), MediaTypeMovie, 949); err != nil {
				t.Fatalf("Follow: %v", err)
			}
			if err := tc.withdraw(svc); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if followers, _ := store.ListTitleFollowers(context.Background(), MediaTypeMovie, 949); len(followers) != 0 {
				t.Fatalf("followers after %s = %+v, want none", tc.name, followers)
			}
		})
	}
}

func TestSearchMarksFollowedAndOwnTitles(t *testing.T) {
	store := newFakeStore()
	activeRequestFor(store, 949)
	own := &Request{ID: "req-own", MediaType: MediaTypeMovie, TMDBID: 950, Status: StatusPending, Outcome: OutcomeActive,
		RequestedByUserID: 1, RequestedByProfileID: "profile-1"}
	store.requests[own.ID] = own
	store.active[MediaTypeMovie][950] = own
	other := &Request{ID: "req-other", MediaType: MediaTypeMovie, TMDBID: 951, Status: StatusPending, Outcome: OutcomeActive,
		RequestedByUserID: 3, RequestedByProfileID: "someone"}
	store.requests[other.ID] = other
	store.active[MediaTypeMovie][951] = other
	if err := store.FollowTitle(context.Background(), MediaTypeMovie, 949, testViewer(1)); err != nil {
		t.Fatal(err)
	}
	svc := newTestServiceWithTMDB(store, &fakeTMDBClient{page: &tmdb.MediaPage{Page: 1, Results: []tmdb.MediaResult{
		{ID: 949, MediaType: "movie", Title: "Heat"},
		{ID: 950, MediaType: "movie", Title: "Ronin"},
		{ID: 951, MediaType: "movie", Title: "Thief"},
	}}})

	page, err := svc.Search(context.Background(), testViewer(1), "heat", MediaTypeMovie, 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	following := map[int]bool{}
	for _, r := range page.Results {
		following[r.TMDBID] = r.Request.Following
	}
	if !following[949] || !following[950] || following[951] {
		t.Fatalf("following = %v, want the followed and the own title, not the other account's", following)
	}
}

func TestNotifyFulfilledTellsFollowersAndClearsThem(t *testing.T) {
	store := newFakeStore()
	store.requests["req1"] = completedRequestFixture("req1", 42)
	store.unnotified = []string{"req1"}
	store.seedFollow(MediaTypeMovie, 42, Viewer{UserID: 3, ProfileID: "follower-profile"})
	notifier := &fakeNotifier{}
	svc := NewService(store, &fakeTMDBClient{}, presentMovie(42))
	svc.SetFulfillmentNotifier(notifier)

	svc.notifyFulfilledPending(context.Background())

	if len(notifier.followers) != 1 || len(notifier.followers[0]) != 1 || notifier.followers[0][0] != (Follower{UserID: 3, ProfileID: "follower-profile"}) {
		t.Fatalf("followers handed to the notifier = %+v, want the one follower", notifier.followers)
	}
	if followers, _ := store.ListTitleFollowers(context.Background(), MediaTypeMovie, 42); len(followers) != 0 {
		t.Fatalf("followers after notifying = %+v, want cleared", followers)
	}
}

func TestNotifyFulfilledKeepsFollowersWhenDispatchFails(t *testing.T) {
	store := newFakeStore()
	store.requests["req1"] = completedRequestFixture("req1", 42)
	store.unnotified = []string{"req1"}
	store.seedFollow(MediaTypeMovie, 42, Viewer{UserID: 3, ProfileID: "follower-profile"})
	svc := NewService(store, &fakeTMDBClient{}, presentMovie(42))
	svc.SetFulfillmentNotifier(&fakeNotifier{err: errors.New("dispatch failed")})

	svc.notifyFulfilledPending(context.Background())

	if followers, _ := store.ListTitleFollowers(context.Background(), MediaTypeMovie, 42); len(followers) != 1 {
		t.Fatalf("followers after a failed dispatch = %+v, want kept for the retry", followers)
	}
}

func TestFollowsDatabase(t *testing.T) {
	repo, pool := lifecycleTestRepository(t)
	ctx := t.Context()
	// The schema copy has no user_profiles foreign key; the migration's key is
	// exercised by the migrated database, not here.
	if _, err := pool.Exec(ctx, `CREATE TABLE media_request_follows (LIKE public.media_request_follows INCLUDING ALL)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.FollowTitle(ctx, MediaTypeMovie, 949, Viewer{UserID: 1, ProfileID: "profile-a"}); !errors.Is(err, ErrNotRequested) {
		t.Fatalf("follow with no open request: err = %v, want ErrNotRequested", err)
	}
	insertLifecycleRequest(t, repo, "movie-949", 5, 949, StatusPending)
	insertLifecycleRequest(t, repo, "series-949", 5, 1, StatusPending)
	if _, err := pool.Exec(ctx, `UPDATE media_requests SET media_type = 'series', tmdb_id = 949 WHERE id = 'series-949'`); err != nil {
		t.Fatal(err)
	}
	a := Viewer{UserID: 1, ProfileID: "profile-a"}
	b := Viewer{UserID: 2, ProfileID: "profile-b"}
	for range 2 {
		if err := repo.FollowTitle(ctx, MediaTypeMovie, 949, a); err != nil {
			t.Fatalf("follow (idempotent): %v", err)
		}
	}
	if err := repo.FollowTitle(ctx, MediaTypeMovie, 949, b); err != nil {
		t.Fatal(err)
	}
	if err := repo.FollowTitle(ctx, MediaTypeSeries, 949, a); err != nil {
		t.Fatal(err)
	}

	followers, err := repo.ListTitleFollowers(ctx, MediaTypeMovie, 949)
	if err != nil {
		t.Fatal(err)
	}
	if len(followers) != 2 {
		t.Fatalf("movie followers = %+v, want two (the series follow is a different title)", followers)
	}
	followed, err := repo.FollowedTitles(ctx, MediaTypeMovie, []int{949, 950}, "profile-a")
	if err != nil {
		t.Fatal(err)
	}
	if !followed[949] || followed[950] {
		t.Fatalf("followed = %v, want only 949", followed)
	}

	if err := repo.ClearTitleFollowers(ctx, MediaTypeMovie, 949, []string{"profile-a"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnfollowTitle(ctx, MediaTypeMovie, 949, "profile-b"); err != nil {
		t.Fatal(err)
	}
	if followers, _ := repo.ListTitleFollowers(ctx, MediaTypeMovie, 949); len(followers) != 0 {
		t.Fatalf("movie followers after clear and unfollow = %+v, want none", followers)
	}
	if followers, _ := repo.ListTitleFollowers(ctx, MediaTypeSeries, 949); len(followers) != 1 {
		t.Fatalf("series followers = %+v, want the one untouched follow", followers)
	}
	if err := repo.ForgetTitleFollows(ctx, MediaTypeSeries, 949); err != nil {
		t.Fatal(err)
	}
	if followers, _ := repo.ListTitleFollowers(ctx, MediaTypeSeries, 949); len(followers) != 0 {
		t.Fatalf("series followers after forgetting = %+v, want none", followers)
	}
}
