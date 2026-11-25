package apiv2

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	"github.com/Silo-Server/silo-server/internal/userstore"
)

// An administrator's read of one profile's viewer state: the watch progress
// (including what the profile marked watched without playing) and the
// favorites the profile's own listProgress and listFavorites report. Library
// maintenance tools, which decide what is safe to remove, need it for every
// profile on the server; playback history alone misses manual marks and
// favorites. Both listings read the same store seams as the profile's own
// operations, so unwatch tombstones and removed titles behave identically;
// the acting administrator's access filter applies instead of the profile's.

const (
	opListAdminUserProfileProgress  = "listAdminUserProfileProgress"
	opListAdminUserProfileFavorites = "listAdminUserProfileFavorites"
)

// AdminProfileProgressInput is the listAdminUserProfileProgress query.
type AdminProfileProgressInput struct {
	ID        ID     `path:"id" doc:"The account" example:"7"`
	ProfileID string `path:"profile_id" minLength:"1" maxLength:"128" doc:"A profile of the account" example:"p-owner"`
	Status    string `query:"status" enum:"in_progress,completed" doc:"Only entries in this state; absent lists every entry" example:"completed"`
	LibraryID ID     `query:"library_id" doc:"Only entries whose item is in this library" example:"1"`
	LimitParam
	Cursor string `query:"cursor" doc:"Opaque cursor from page.next_cursor" example:"eyJvZmZzZXQiOjUwfQ"`
}

// AdminProfileFavoritesInput is the listAdminUserProfileFavorites query.
type AdminProfileFavoritesInput struct {
	ID        ID     `path:"id" doc:"The account" example:"7"`
	ProfileID string `path:"profile_id" minLength:"1" maxLength:"128" doc:"A profile of the account" example:"p-owner"`
	LimitParam
	Cursor string `query:"cursor" doc:"Opaque cursor from page.next_cursor" example:"eyJvIjo1MH0"`
}

// AdminProfileFavoriteCollection is the named envelope the contract carries:
// the profile's favorite entries, newest favorite first.
type AdminProfileFavoriteCollection struct {
	Collection[FavoriteEntry]
}

// AdminProfileFavoriteCollectionOutput is the listAdminUserProfileFavorites
// response.
type AdminProfileFavoriteCollectionOutput struct {
	Body AdminProfileFavoriteCollection
}

func registerAdminProfileState(reg *Registry) {
	cursors := NewCursors(reg.deps.CursorSecret)

	progress := adminAccountOperation(http.MethodGet, adminProfilePath+"/progress", opListAdminUserProfileProgress, false)
	progress.Summary = "List one profile's watch progress, newest change first."
	progress.Description = "The entries listProgress reports to the profile itself: completed entries are what the profile watched, by playing or by marking it watched, and an item marked unwatched has no entry. Items the acting administrator may not see and removed titles are omitted. 404 not_found when the profile does not belong to the account."
	Register(reg, progress, func(ctx context.Context, in *AdminProfileProgressInput) (*ProgressCollectionOutput, error) {
		return reg.listAdminProfileProgress(ctx, cursors, in)
	})

	favorites := adminAccountOperation(http.MethodGet, adminProfilePath+"/favorites", opListAdminUserProfileFavorites, false)
	favorites.Summary = "List one profile's favorites, newest favorite first."
	favorites.Description = "The entries listFavorites reports to the profile itself, as item ids with the time each became a favorite. Items the acting administrator may not see and removed titles are omitted. 404 not_found when the profile does not belong to the account."
	Register(reg, favorites, func(ctx context.Context, in *AdminProfileFavoritesInput) (*AdminProfileFavoriteCollectionOutput, error) {
		return reg.listAdminProfileFavorites(ctx, cursors, in)
	})
}

// adminProfileStateTarget resolves the addressed account and confirms the
// profile is one of its profiles: 404 for an unknown account or a profile
// of another account, so the read never reaches a store it should not.
func (reg *Registry) adminProfileStateTarget(ctx context.Context, rawID ID, profileID string) (int, *Problem) {
	svc, id, p := reg.adminProfilesTarget(ctx, rawID)
	if p != nil {
		return 0, p
	}
	if _, err := svc.GetProfile(ctx, id, profileID); err != nil {
		return 0, profileProblem(err)
	}
	return id, nil
}

// adminProfileStateSecurity binds a cursor to the target profile and to the
// acting administrator's access filter, which decides what the page omits.
func adminProfileStateSecurity(ctx context.Context, userID int, profileID string) string {
	return strconv.Itoa(userID) + "/" + profileID + "/" + viewerScopeDigest(ctx)
}

func (reg *Registry) listAdminProfileProgress(ctx context.Context, cursors *Cursors, in *AdminProfileProgressInput) (*ProgressCollectionOutput, error) {
	if reg.deps.Progress == nil {
		return nil, unavailable("progress")
	}
	userID, p := reg.adminProfileStateTarget(ctx, in.ID, in.ProfileID)
	if p != nil {
		return nil, p
	}
	libraryID := 0
	if in.LibraryID != "" {
		n, err := intOfID(in.LibraryID)
		if err != nil || n <= 0 {
			return nil, NewProblem(TypeValidationFailed, "The request did not pass validation; see errors.").
				WithErrors(ProblemError{Location: locationQueryLibraryID, Code: codeInvalid, Detail: detailLibraryIDInvalid})
		}
		libraryID = n
	}
	scope := CursorScope{
		OperationID: opListAdminUserProfileProgress,
		Security:    adminProfileStateSecurity(ctx, userID, in.ProfileID),
		Filter:      in.Status + "|" + string(in.LibraryID),
		Sort:        "-updated_at,-media_item_id",
		Tiebreaker:  "media_item_id",
	}
	var after *userstore.ProgressKey
	if in.Cursor != "" {
		var pos progressPosition
		if p := cursors.Decode(scope, in.Cursor, &pos); p != nil {
			return nil, p
		}
		after = &userstore.ProgressKey{UpdatedAt: pos.UpdatedAt, MediaItemID: pos.MediaItemID}
	}
	entries, hasMore, err := reg.deps.Progress.ListProgressPage(ctx, userID, in.ProfileID, in.Status, libraryID, after, in.Limit)
	if err != nil {
		return nil, serviceProblem(err)
	}
	next := ""
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		next, err = cursors.Encode(scope, progressPosition{UpdatedAt: last.UpdatedAt, MediaItemID: last.MediaItemID})
		if err != nil {
			return nil, NewProblem(TypeInternalError, "An unexpected error occurred.")
		}
	}
	items := make([]ProgressEntry, 0, len(entries))
	for _, e := range entries {
		entry, p := progressEntryOf(e)
		if p != nil {
			return nil, p
		}
		items = append(items, entry)
	}
	return &ProgressCollectionOutput{Body: ProgressCollection{Collection: Paginated(items, next)}}, nil
}

func (reg *Registry) listAdminProfileFavorites(ctx context.Context, cursors *Cursors, in *AdminProfileFavoritesInput) (*AdminProfileFavoriteCollectionOutput, error) {
	if reg.deps.PersonalLists == nil {
		return nil, unavailable("favorites")
	}
	userID, p := reg.adminProfileStateTarget(ctx, in.ID, in.ProfileID)
	if p != nil {
		return nil, p
	}
	viewer := handlers.PersonalListViewer{
		UserID: userID, ProfileID: in.ProfileID, Access: handlers.AccessFilterFromContext(ctx, ""),
	}
	scope := CursorScope{
		OperationID: opListAdminUserProfileFavorites,
		Security:    adminProfileStateSecurity(ctx, userID, in.ProfileID),
		Sort:        "-added_at,-item_id",
		Tiebreaker:  tiebreakerItemID,
	}
	after, p := decodeListKey(cursors, scope, in.Cursor)
	if p != nil {
		return nil, p
	}
	// One probe row past the page decides has_more, as listFavorites does.
	entries, _, err := reg.deps.PersonalLists.ListFavoritesPage(ctx, viewer, after, in.Limit+1)
	if err != nil {
		return nil, serviceProblem(err)
	}
	next := ""
	if len(entries) > in.Limit {
		entries = entries[:in.Limit]
		last := entries[len(entries)-1]
		if next, err = cursors.Encode(scope, personalListPosition{AddedAt: last.AddedAt, MediaItemID: last.MediaItemID}); err != nil {
			return nil, NewProblem(TypeInternalError, "An unexpected error occurred.")
		}
	}
	items := make([]FavoriteEntry, 0, len(entries))
	for _, e := range entries {
		added, p := storeInstant(e.AddedAt)
		if p != nil {
			return nil, p
		}
		items = append(items, FavoriteEntry{ItemID: ID(e.MediaItemID), AddedAt: added})
	}
	return &AdminProfileFavoriteCollectionOutput{Body: AdminProfileFavoriteCollection{Collection: Paginated(items, next)}}, nil
}
