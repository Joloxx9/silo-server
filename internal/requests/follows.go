package requests

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Following a title: a profile that finds a title someone else has already
// requested can ask to be notified when it becomes available, instead of
// requesting it again. A follow belongs to the title, not to one request, so it
// survives the request failing and being retried or requested again. It is
// cleared once the fulfilled notification has gone out, and when the request
// is declined or withdrawn: the title is then no longer on its way, and the
// follower can request it themselves. The requester is always notified and
// never needs a follow.

// Follower is a profile waiting to hear that a requested title is available.
type Follower struct {
	UserID    int
	ProfileID string
}

// Follow records that the viewer's profile wants to hear when the title
// becomes available. The title must have an open request. Following a title
// the viewer requested is a no-op.
func (s *Service) Follow(ctx context.Context, viewer Viewer, mediaType MediaType, tmdbID int) (RequestState, error) {
	if err := validateViewer(viewer); err != nil {
		return RequestState{}, err
	}
	if strings.TrimSpace(viewer.ProfileID) == "" {
		return RequestState{}, ErrForbidden
	}
	if err := s.ensureRequestsEnabled(ctx); err != nil {
		return RequestState{}, err
	}
	if err := s.ensureViewerRequestsAllowed(ctx, viewer.UserID); err != nil {
		return RequestState{}, err
	}
	if blocked, err := s.userLimitBlocked(ctx, viewer.UserID); err != nil {
		return RequestState{}, err
	} else if blocked {
		return RequestState{}, ErrUserBlocked
	}
	mediaType, err := normalizeMediaType(mediaType)
	if err != nil {
		return RequestState{}, err
	}
	if tmdbID <= 0 {
		return RequestState{}, fmt.Errorf("%w: tmdb id is required", ErrInvalidInput)
	}
	// Following reaches the same titles requesting does, so the profile's
	// rating ceiling applies to it too.
	if err := s.ensureCreateAllowedByCeiling(ctx, viewer, CreateRequestInput{MediaType: mediaType, TMDBID: tmdbID}); err != nil {
		return RequestState{}, err
	}
	// An open request is what makes a title followable: a series partly in
	// the library can have one for its missing seasons.
	active, err := s.store.ListActiveByTMDB(ctx, mediaType, []int{tmdbID})
	if err != nil {
		return RequestState{}, err
	}
	req := active[tmdbID]
	if req == nil {
		return RequestState{}, ErrNotRequested
	}
	if req.RequestedByProfileID != viewer.ProfileID {
		if err := s.store.FollowTitle(ctx, mediaType, tmdbID, viewer); err != nil {
			return RequestState{}, err
		}
	}
	state := activeRequestState(viewer, req)
	state.Following = true
	return state, nil
}

// Unfollow removes the viewer's follow. It succeeds whether or not the profile
// followed the title, and whatever state the title's request is in.
func (s *Service) Unfollow(ctx context.Context, viewer Viewer, mediaType MediaType, tmdbID int) error {
	if err := validateViewer(viewer); err != nil {
		return err
	}
	if strings.TrimSpace(viewer.ProfileID) == "" {
		return ErrForbidden
	}
	mediaType, err := normalizeMediaType(mediaType)
	if err != nil {
		return err
	}
	if tmdbID <= 0 {
		return fmt.Errorf("%w: tmdb id is required", ErrInvalidInput)
	}
	return s.store.UnfollowTitle(ctx, mediaType, tmdbID, viewer.ProfileID)
}

// forgetFollowsAfterWithdrawal clears a withdrawn request's title follows.
// Best effort: a leftover follow only fires if the title is requested again
// and arrives.
func (s *Service) forgetFollowsAfterWithdrawal(ctx context.Context, req *Request) {
	if req == nil {
		return
	}
	if err := s.store.ForgetTitleFollows(ctx, req.MediaType, req.TMDBID); err != nil {
		slog.WarnContext(ctx, "requests: clearing follows after withdrawal failed", "component", "requests",
			"request_id", req.ID, "err", err)
	}
}

// followedTitles reports which of the titles with an active request the viewer
// is waiting on, either as the requesting profile or as a follower.
func (s *Service) followedTitles(ctx context.Context, viewer Viewer, mediaType MediaType, active map[int]*Request) (map[int]bool, error) {
	out := map[int]bool{}
	var others []int
	for tmdbID, req := range active {
		if req == nil {
			continue
		}
		if req.RequestedByProfileID == viewer.ProfileID {
			out[tmdbID] = true
			continue
		}
		others = append(others, tmdbID)
	}
	if len(others) == 0 || strings.TrimSpace(viewer.ProfileID) == "" {
		return out, nil
	}
	followed, err := s.store.FollowedTitles(ctx, mediaType, others, viewer.ProfileID)
	if err != nil {
		return nil, err
	}
	for tmdbID, ok := range followed {
		if ok {
			out[tmdbID] = true
		}
	}
	return out, nil
}

// FollowTitle inserts the follow only while the title has an open request, in
// the same statement, so a follow cannot land just after the request
// completed and never be told. It answers ErrNotRequested when there is none.
func (r *Repository) FollowTitle(ctx context.Context, mediaType MediaType, tmdbID int, viewer Viewer) error {
	var open bool
	if err := r.pool.QueryRow(ctx, `
		WITH open_request AS (
			SELECT 1 FROM media_requests
			WHERE media_type = $1 AND provider = 'tmdb' AND tmdb_id = $2
			  AND outcome = 'active' AND status <> 'completed'
			LIMIT 1
		), inserted AS (
			INSERT INTO media_request_follows (media_type, tmdb_id, user_id, profile_id)
			SELECT $1, $2, $3, $4 FROM open_request
			ON CONFLICT (media_type, tmdb_id, profile_id) DO NOTHING
		)
		SELECT EXISTS (SELECT 1 FROM open_request)
	`, mediaType, tmdbID, viewer.UserID, viewer.ProfileID).Scan(&open); err != nil {
		return fmt.Errorf("follow title: %w", err)
	}
	if !open {
		return ErrNotRequested
	}
	return nil
}

// ForgetTitleFollows removes every follow on the title.
func (r *Repository) ForgetTitleFollows(ctx context.Context, mediaType MediaType, tmdbID int) error {
	if _, err := r.pool.Exec(ctx, `
		DELETE FROM media_request_follows WHERE media_type = $1 AND tmdb_id = $2
	`, mediaType, tmdbID); err != nil {
		return fmt.Errorf("forget title follows: %w", err)
	}
	return nil
}

func (r *Repository) UnfollowTitle(ctx context.Context, mediaType MediaType, tmdbID int, profileID string) error {
	if _, err := r.pool.Exec(ctx, `
		DELETE FROM media_request_follows
		WHERE media_type = $1 AND tmdb_id = $2 AND profile_id = $3
	`, mediaType, tmdbID, profileID); err != nil {
		return fmt.Errorf("unfollow title: %w", err)
	}
	return nil
}

func (r *Repository) FollowedTitles(ctx context.Context, mediaType MediaType, tmdbIDs []int, profileID string) (map[int]bool, error) {
	out := map[int]bool{}
	if len(tmdbIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT tmdb_id FROM media_request_follows
		WHERE media_type = $1 AND tmdb_id = ANY($2) AND profile_id = $3
	`, mediaType, tmdbIDs, profileID)
	if err != nil {
		return nil, fmt.Errorf("list followed titles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tmdbID int
		if err := rows.Scan(&tmdbID); err != nil {
			return nil, err
		}
		out[tmdbID] = true
	}
	return out, rows.Err()
}

func (r *Repository) ListTitleFollowers(ctx context.Context, mediaType MediaType, tmdbID int) ([]Follower, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, profile_id FROM media_request_follows
		WHERE media_type = $1 AND tmdb_id = $2
		ORDER BY created_at, profile_id
	`, mediaType, tmdbID)
	if err != nil {
		return nil, fmt.Errorf("list title followers: %w", err)
	}
	defer rows.Close()
	var out []Follower
	for rows.Next() {
		var f Follower
		if err := rows.Scan(&f.UserID, &f.ProfileID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *Repository) ClearTitleFollowers(ctx context.Context, mediaType MediaType, tmdbID int, profileIDs []string) error {
	if len(profileIDs) == 0 {
		return nil
	}
	if _, err := r.pool.Exec(ctx, `
		DELETE FROM media_request_follows
		WHERE media_type = $1 AND tmdb_id = $2 AND profile_id = ANY($3)
	`, mediaType, tmdbID, profileIDs); err != nil {
		return fmt.Errorf("clear title followers: %w", err)
	}
	return nil
}
