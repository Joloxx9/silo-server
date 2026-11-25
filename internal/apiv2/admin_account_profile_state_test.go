package apiv2

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Silo-Server/silo-server/internal/userstore"
)

func adminProfileStateHandler(progress *fakeProgress, lists *fakePersonalLists) http.Handler {
	deps := requestDeps(fixtureRequests())
	deps.AdminAccounts = fixtureAdminAccounts()
	deps.Profiles = &fakeProfiles{view: fixtureProfileView()}
	deps.Progress = progress
	deps.PersonalLists = lists
	return NewHandler(deps)
}

func TestAdminProfileStateRequiresAnAdministrator(t *testing.T) {
	progress := &fakeProgress{}
	lists := &fakePersonalLists{}
	h := adminProfileStateHandler(progress, lists)
	for _, path := range []string{adminProfilesBase + "/p-owner/progress", adminProfilesBase + "/p-owner/favorites"} {
		requireProblem(t, do(t, h, http.MethodGet, path, "", bearer(memberToken)), TypePermissionDenied)
	}
	if len(progress.calls) != 0 || len(lists.viewers) != 0 {
		t.Fatal("a member reached the viewer-state seams")
	}
}

// A profile of another account is a 404 before any store read, so an
// administrator cannot pair one account's id with another's profile.
func TestAdminProfileStateRejectsAProfileOfAnotherAccount(t *testing.T) {
	progress := &fakeProgress{}
	lists := &fakePersonalLists{}
	h := adminProfileStateHandler(progress, lists)
	for _, path := range []string{adminProfilesBase + "/p-elsewhere/progress", adminProfilesBase + "/p-elsewhere/favorites"} {
		requireProblem(t, do(t, h, http.MethodGet, path, "", actingRequestAdmin), TypeNotFound)
	}
	if len(progress.calls) != 0 || len(lists.viewers) != 0 {
		t.Fatal("a foreign profile reached the viewer-state seams")
	}
}

func TestAdminProfileProgressPagesTheAddressedProfile(t *testing.T) {
	progress := &fakeProgress{entries: []userstore.WatchProgress{
		{ProfileID: "p-owner", MediaItemID: "movie-a", Completed: true, UpdatedAt: "2026-03-03T00:00:00Z"},
		{ProfileID: "p-owner", MediaItemID: "movie-b", Completed: true, UpdatedAt: "2026-03-02T00:00:00Z"},
		{ProfileID: "p-owner", MediaItemID: "movie-c", PositionSeconds: 60, UpdatedAt: "2026-03-01T00:00:00Z"},
		{ProfileID: "p-kid", MediaItemID: "movie-d", Completed: true, UpdatedAt: "2026-03-04T00:00:00Z"},
	}}
	h := adminProfileStateHandler(progress, &fakePersonalLists{})

	var page struct {
		Items []ProgressEntry `json:"items"`
		Page  struct {
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		} `json:"page"`
	}
	rec := do(t, h, http.MethodGet, adminProfilesBase+"/p-owner/progress?status=completed&limit=1", "", actingRequestAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("first page: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].MediaItemID != "movie-a" || !page.Items[0].Completed || !page.Page.HasMore {
		t.Fatalf("first page = %+v", page)
	}

	rec = do(t, h, http.MethodGet, adminProfilesBase+"/p-owner/progress?status=completed&limit=1&cursor="+page.Page.NextCursor, "", actingRequestAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("second page: %d %s", rec.Code, rec.Body)
	}
	page.Items = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].MediaItemID != "movie-b" || page.Page.HasMore {
		t.Fatalf("second page = %+v", page)
	}

	// The cursor is bound to the profile and the filter it was minted for.
	requireProblem(t, do(t, h, http.MethodGet, adminProfilesBase+"/p-owner/progress?limit=1&cursor="+page.Page.NextCursor, "", actingRequestAdmin), TypeInvalidCursor)
}

func TestAdminProfileFavoritesListEntriesOfTheAddressedProfile(t *testing.T) {
	lists := &fakePersonalLists{favorites: []userstore.Favorite{
		{ProfileID: "p-owner", MediaItemID: "movie-a", AddedAt: "2026-03-02T00:00:00Z"},
		{ProfileID: "p-owner", MediaItemID: "series-b", AddedAt: "2026-03-01T00:00:00Z"},
		{ProfileID: "p-kid", MediaItemID: "movie-c", AddedAt: "2026-03-03T00:00:00Z"},
	}}
	h := adminProfileStateHandler(&fakeProgress{}, lists)

	rec := do(t, h, http.MethodGet, adminProfilesBase+"/p-owner/favorites?limit=1", "", actingRequestAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("first page: %d %s", rec.Code, rec.Body)
	}
	var page struct {
		Items []FavoriteEntry `json:"items"`
		Page  struct {
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ItemID != "movie-a" || !page.Page.HasMore {
		t.Fatalf("first page = %+v", page)
	}
	if v := lists.viewers[0]; v.UserID != 7 || v.ProfileID != "p-owner" {
		t.Fatalf("viewer = %+v, want account 7 profile p-owner", v)
	}

	rec = do(t, h, http.MethodGet, adminProfilesBase+"/p-owner/favorites?limit=1&cursor="+page.Page.NextCursor, "", actingRequestAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("second page: %d %s", rec.Code, rec.Body)
	}
	page.Items = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ItemID != "series-b" || page.Page.HasMore {
		t.Fatalf("second page = %+v", page)
	}
}
