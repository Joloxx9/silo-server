package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
	"github.com/Silo-Server/silo-server/internal/taskmanager"
)

// PrefetchPersonCreditsTaskKey identifies the nightly filmography prefetch.
const PrefetchPersonCreditsTaskKey = "prefetch_person_credits"

const (
	// prefetchPersonCreditsBudget caps the people one run asks TMDB about, so
	// a large library is covered over several nights.
	prefetchPersonCreditsBudget = 2000
	// prefetchPersonCreditsMaxFailures stops a run that keeps failing (TMDB
	// down, key revoked) instead of spending the night on errors.
	prefetchPersonCreditsMaxFailures = 25
)

type personCreditsDue interface {
	PeopleDueForRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]int, error)
}

type personCreditsRefresher interface {
	RefreshPersonCredits(ctx context.Context, personID int) error
}

// PrefetchPersonCreditsTask fetches the TMDB filmographies of people credited
// in the library ahead of time, so their person pages open from the database
// instead of waiting on TMDB. People credited on the most titles go first;
// one TMDB request per person, one person at a time.
type PrefetchPersonCreditsTask struct {
	due       personCreditsDue
	refresher personCreditsRefresher
}

func NewPrefetchPersonCreditsTask(due personCreditsDue, refresher personCreditsRefresher) *PrefetchPersonCreditsTask {
	return &PrefetchPersonCreditsTask{due: due, refresher: refresher}
}

func (t *PrefetchPersonCreditsTask) Key() string  { return PrefetchPersonCreditsTaskKey }
func (t *PrefetchPersonCreditsTask) Name() string { return "Prefetch Person Filmographies" }
func (t *PrefetchPersonCreditsTask) Description() string {
	return "Fetches the TMDB filmographies of people in your libraries ahead of time, so person pages open without waiting."
}
func (t *PrefetchPersonCreditsTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryMetadata
}
func (t *PrefetchPersonCreditsTask) IsHidden() bool { return false }
func (t *PrefetchPersonCreditsTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{{
		Type:         taskmanager.TriggerTypeDaily,
		TimeOfDay:    "03:30",
		MaxRuntimeMs: int64(2 * time.Hour / time.Millisecond),
	}}
}

// StartSpread keeps servers that share the TMDB project key from starting
// their prefetch at the same minute.
func (t *PrefetchPersonCreditsTask) StartSpread() time.Duration { return time.Hour }

func (t *PrefetchPersonCreditsTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	if t == nil || t.due == nil || t.refresher == nil {
		progress.Report(100, "Filmography prefetch is not configured")
		return nil
	}
	people, err := t.due.PeopleDueForRefresh(ctx, time.Now().Add(-tmdb.PersonCreditsFreshFor), prefetchPersonCreditsBudget)
	if err != nil {
		return err
	}
	if len(people) == 0 {
		progress.Report(100, "Every filmography is up to date")
		return nil
	}
	fetched, failed := 0, 0
	for i, personID := range people {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.refresher.RefreshPersonCredits(ctx, personID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A person TMDB no longer has is not worth stopping for.
			if !errors.Is(err, tmdb.ErrNotFound) {
				failed++
			}
			if failed >= prefetchPersonCreditsMaxFailures {
				return fmt.Errorf("prefetch person filmographies: stopped after %d failures (fetched %d): %w", failed, fetched, err)
			}
			continue
		}
		fetched++
		if i%20 == 0 {
			progress.Report(float64(i)*100/float64(len(people)), fmt.Sprintf("Fetched %d of %d filmographies", fetched, len(people)))
		}
	}
	progress.Report(100, fmt.Sprintf("Fetched %d filmographies", fetched))
	return nil
}
