package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Silo-Server/silo-server/internal/requests"
	"github.com/Silo-Server/silo-server/internal/taskmanager"
	"github.com/jackc/pgx/v5/pgxpool"
)

// requestReconcileAdvisoryLock spells "SILORQRC".
const requestReconcileAdvisoryLock int64 = 0x53494C4F52515243

type RequestReconciler interface {
	ReconcileRequests(ctx context.Context, limit int) (requests.ReconcileResult, error)
}

// ReconcileRequestsTask moves in-flight media requests forward. Every API
// process runs the task manager, so an advisory lock lets one server run each
// pass; the others skip. The per-request submission claim already prevents a
// double submission, so the lock only saves the duplicate router status calls
// and presence lookups.
type ReconcileRequestsTask struct {
	reconciler RequestReconciler
	limit      int
	lock       clusterLock
}

// NewReconcileRequestsTask constructs the task. A nil pool runs without the
// cluster lock.
func NewReconcileRequestsTask(reconciler RequestReconciler, limit int, pool *pgxpool.Pool) *ReconcileRequestsTask {
	if limit <= 0 {
		limit = 100
	}
	t := &ReconcileRequestsTask{reconciler: reconciler, limit: limit}
	if pool != nil {
		t.lock = advisoryClusterLock{pool: pool, key: requestReconcileAdvisoryLock}
	}
	return t
}

func (t *ReconcileRequestsTask) Key() string  { return "reconcile_requests" }
func (t *ReconcileRequestsTask) Name() string { return "Reconcile Requests" }
func (t *ReconcileRequestsTask) Description() string {
	return "Checks approved and active media requests against Radarr, Sonarr, and the Silo catalog"
}
func (t *ReconcileRequestsTask) Category() taskmanager.TaskCategory {
	return taskmanager.TaskCategoryLibrary
}
func (t *ReconcileRequestsTask) IsHidden() bool { return true }

func (t *ReconcileRequestsTask) DefaultTriggers() []taskmanager.TriggerConfig {
	return []taskmanager.TriggerConfig{
		{Type: taskmanager.TriggerTypeInterval, IntervalMs: 5 * 60 * 1000},
	}
}

func (t *ReconcileRequestsTask) Execute(ctx context.Context, progress taskmanager.ProgressReporter) error {
	progress.Report(0, "Reconciling media requests")
	if t.reconciler == nil {
		progress.Report(100, "Request reconciliation unavailable")
		return nil
	}
	if t.lock != nil {
		release, acquired, err := t.lock.TryAcquire(ctx)
		if err != nil {
			return fmt.Errorf("acquiring request reconcile lock: %w", err)
		}
		if !acquired {
			progress.Report(100, "Another server is reconciling media requests")
			return nil
		}
		defer release()
	}
	result, err := t.reconciler.ReconcileRequests(ctx, t.limit)
	if err != nil {
		return fmt.Errorf("reconcile media requests: %w", err)
	}
	if data, err := json.Marshal(result); err == nil {
		progress.SetResultData(data)
	}
	progress.Report(100, "Request reconciliation complete")
	return nil
}
