package notifications

import (
	"context"
	"errors"
	"testing"

	"github.com/Silo-Server/silo-server/internal/requests"
)

type fakeFulfillmentBackend struct {
	disabled    map[string]bool
	failFor     string
	deliveries  []Delivery
	channelPost int
}

func (f *fakeFulfillmentBackend) PostServerChannelRequestEvent(context.Context, string, RequestEventInfo) {
	f.channelPost++
}

func (f *fakeFulfillmentBackend) notificationsEnabled(_ context.Context, profileID string) (bool, error) {
	return !f.disabled[profileID], nil
}

func (f *fakeFulfillmentBackend) dispatchFulfilled(_ context.Context, delivery Delivery) error {
	if delivery.ProfileID == f.failFor {
		return errors.New("dispatch failed")
	}
	f.deliveries = append(f.deliveries, delivery)
	return nil
}

func fulfilledRequest(followers ...requests.Follower) requests.Request {
	return requests.Request{
		ID: "req-1", MediaType: requests.MediaTypeMovie, TMDBID: 949, Title: "Heat",
		RequestedByUserID: 1, RequestedByProfileID: "requester", Followers: followers,
	}
}

func TestNotifyFulfilledTellsRequesterAndFollowers(t *testing.T) {
	backend := &fakeFulfillmentBackend{disabled: map[string]bool{"muted": true}}
	notifier := &RequestFulfillmentNotifier{backend: backend}

	err := notifier.NotifyFulfilled(context.Background(), fulfilledRequest(
		requests.Follower{UserID: 2, ProfileID: "follower"},
		requests.Follower{UserID: 1, ProfileID: "requester"}, // a leftover follow by the requester
		requests.Follower{UserID: 3, ProfileID: "muted"},
	), "movie-tmdb-949")
	if err != nil {
		t.Fatalf("NotifyFulfilled: %v", err)
	}
	if len(backend.deliveries) != 2 {
		t.Fatalf("deliveries = %+v, want the requester and the one unmuted follower", backend.deliveries)
	}
	requester, follower := backend.deliveries[0], backend.deliveries[1]
	if requester.ProfileID != "requester" || parseRequestFlags(requester.ReasonFlags).Follower {
		t.Fatalf("first delivery = %+v, want the requester's own copy", requester)
	}
	if follower.ProfileID != "follower" || follower.UserID != 2 || !parseRequestFlags(follower.ReasonFlags).Follower {
		t.Fatalf("second delivery = %+v, want the follower's copy marked as such", follower)
	}
	if flags := parseRequestFlags(follower.ReasonFlags); flags.RequestID != "req-1" || flags.TMDBID != 949 {
		t.Fatalf("follower flags = %+v, want the request identity", flags)
	}
	if backend.channelPost != 1 {
		t.Fatalf("channel posts = %d, want 1", backend.channelPost)
	}
}

// Profile ids repeat across accounts, so a follower on another account whose
// profile id matches the requester's is still a separate recipient.
func TestNotifyFulfilledKeysRecipientsByAccount(t *testing.T) {
	backend := &fakeFulfillmentBackend{}
	notifier := &RequestFulfillmentNotifier{backend: backend}
	req := fulfilledRequest(requests.Follower{UserID: 2, ProfileID: "default"})
	req.RequestedByProfileID = "default"

	if err := notifier.NotifyFulfilled(context.Background(), req, "movie-tmdb-949"); err != nil {
		t.Fatalf("NotifyFulfilled: %v", err)
	}
	if len(backend.deliveries) != 2 || backend.deliveries[1].UserID != 2 || !parseRequestFlags(backend.deliveries[1].ReasonFlags).Follower {
		t.Fatalf("deliveries = %+v, want the requester and the other account's follower", backend.deliveries)
	}
}

// A failed recipient makes the caller retry the whole request; the community
// channel must not be posted until an attempt reaches everyone.
func TestNotifyFulfilledPostsChannelOnlyAfterEveryRecipient(t *testing.T) {
	backend := &fakeFulfillmentBackend{failFor: "follower"}
	notifier := &RequestFulfillmentNotifier{backend: backend}

	err := notifier.NotifyFulfilled(context.Background(), fulfilledRequest(requests.Follower{UserID: 2, ProfileID: "follower"}), "movie-tmdb-949")
	if err == nil {
		t.Fatal("NotifyFulfilled succeeded, want the follower's dispatch error")
	}
	if backend.channelPost != 0 {
		t.Fatalf("channel posts = %d, want none before every recipient is told", backend.channelPost)
	}
}

func TestFulfilledCopyForFollowers(t *testing.T) {
	requester := DeliveryRow{Delivery: Delivery{Type: DeliveryTypeRequestFulfilled, ReasonFlags: []byte(`{"request_id":"req-1"}`)}}
	follower := DeliveryRow{Delivery: Delivery{Type: DeliveryTypeRequestFulfilled, ReasonFlags: []byte(`{"request_id":"req-1","follower":true}`)}}

	if got := BuildNotificationDisplay(requester); got.Title != "Your request is now available" {
		t.Fatalf("requester title = %q", got.Title)
	}
	if got := BuildNotificationDisplay(follower); got.Title != followedTitleAvailable || got.Body == "Your media request has arrived in the library." {
		t.Fatalf("follower display = %+v, want copy that does not claim the request", got)
	}
	if got := requestLine(follower); got != followedTitleAvailable {
		t.Fatalf("follower email line = %q", got)
	}
	if got := discordEmbedAuthorLine(follower); got != "Now available on Silo" {
		t.Fatalf("follower Discord author = %q", got)
	}
}
