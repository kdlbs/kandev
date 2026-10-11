package sqlite

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionSQLiteHolderProcess(t *testing.T) {
	path := os.Getenv("KANDEV_RETENTION_HOLDER_DB")
	if path == "" {
		t.Skip("subprocess-only SQLite writer holder")
	}
	raw, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	tx, err := raw.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	fmt.Println("holder-ready")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}

type retentionBeginObserver struct {
	*Repository
	entered  chan context.Context
	finished chan time.Duration
}

func (r *retentionBeginObserver) RunAgentDeliveryRetentionBatch(ctx context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	started := time.Now()
	r.entered <- ctx
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, options)
	r.finished <- time.Since(started)
	return result, err
}

func TestDeliveryRetentionIntegrationSQLiteIndependentHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "holder.db")
	repo := openRetentionSQLiteRepo(t, path)
	event := seedRetentionCrashEventInRepo(t, repo)
	reader, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	repo.ro = sqlx.NewDb(reader, "sqlite3")
	release := startRetentionSQLiteHolder(t, path)
	defer release()
	observed := &retentionBeginObserver{Repository: repo, entered: make(chan context.Context, 1), finished: make(chan time.Duration, 1)}
	timers := make(chan workerTimer, 2)
	worker := deliveryretention.New(observed, db.NewPool(repo.db, repo.ro), deliveryretention.Options{
		After: func(delay time.Duration) <-chan time.Time {
			timer := workerTimer{delay: delay, fire: make(chan time.Time, 1)}
			timers <- timer
			return timer.fire
		},
	})
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()
	fireWorkerTimer(t, timers, time.Minute)
	batchCtx := <-observed.entered
	<-batchCtx.Done()
	readStart := time.Now()
	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	var payload []byte
	err = repo.ro.GetContext(readCtx, &payload, `SELECT payload FROM agent_delivery_inbox LIMIT 1`)
	cancel()
	if err != nil || string(payload) != string(event.Payload) {
		t.Fatalf("WAL reader while native BEGIN held: payload=%q err=%v", payload, err)
	}
	readDuration := time.Since(readStart)
	stopStart := time.Now()
	stopped := make(chan time.Duration, 1)
	go func() { worker.Stop(); stopped <- time.Since(stopStart) }()
	release()
	var stopDuration time.Duration
	select {
	case stopDuration = <-stopped:
	case <-time.After(7 * time.Second):
		t.Fatal("Stop did not join canceled native BEGIN")
	}
	beginDuration := <-observed.finished
	t.Logf("native_BEGIN_and_settlement=%s canceled_SQL_work=0 reader=%s Stop=%s", beginDuration, readDuration, stopDuration)
	assertRetentionCrashEvidence(t, repo, event, false)
	result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
	if err != nil || result.EventRowsRetired != 1 {
		t.Fatalf("pool reuse after canceled BEGIN: %+v %v", result, err)
	}
}

func startRetentionSQLiteHolder(t *testing.T, path string) func() {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestDeliveryRetentionSQLiteHolderProcess$")
	command.Env = append(os.Environ(), "KANDEV_RETENTION_HOLDER_DB="+path)
	command.Stderr = os.Stderr
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = input.Close()
			if err := command.Wait(); err != nil {
				t.Errorf("SQLite holder exited: %v", err)
			}
		}
	}
	t.Cleanup(release)
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "holder-ready" {
		release()
		t.Fatal("independent SQLite holder did not acquire writer")
	}
	return release
}
