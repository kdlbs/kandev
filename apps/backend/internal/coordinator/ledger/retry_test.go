package ledger

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestRetryContract_DefaultsAreOneSecondAndFourAttempts(t *testing.T) {
	l := New(Deps{})
	if l.retryEvery != time.Second {
		t.Fatalf("retry spacing = %v, want 1s", l.retryEvery)
	}
	if callMaxRetries != 3 {
		t.Fatalf("retries = %d, want 3 (four attempts in all)", callMaxRetries)
	}
}

func TestRetryContract_UnattributedCallIsDroppedOnTheFourthAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.l.retryEvery = time.Second
		f.startWriter()
		before := FailureCount(StageCallUnattributed)
		f.l.Call(testSession, "orphan", "", true)
		synctest.Wait()
		if !f.l.calls.retryPending(testSession) {
			t.Fatal("first attempt did not park the call")
		}
		time.Sleep(2*time.Second + 500*time.Millisecond)
		synctest.Wait()
		if got := FailureCount(StageCallUnattributed); got != before {
			t.Fatalf("dropped before the fourth attempt: %d", got-before)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := FailureCount(StageCallUnattributed) - before; got != 1 {
			t.Fatalf("drops after the fourth attempt = %d, want 1", got)
		}
		if f.l.calls.retryPending(testSession) {
			t.Fatal("dropped call left on the retry list")
		}
	})
}

func (f *fixture) seedFinishedWithoutModel(id string) {
	f.t.Helper()
	now := time.Now().UTC()
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", model, started_at, finished_at, outcome, verdict)
		VALUES (?, ?, 's', ?, 'message', '', ?, ?, 'completed', 'nothing_needed')`, id, f.coord.ID, "st-"+id, now.Add(-time.Hour), now.Add(-time.Minute))
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, agent_type, model, created_at) VALUES ('s', ?, 'agent', 'm-1', ?)`, "st-"+id, now.Add(-30*time.Second))
}

func (f *fixture) modelOf(id string) string {
	f.t.Helper()
	var m string
	if err := f.db.Get(&m, f.db.Rebind(`SELECT model FROM coordinator_turns WHERE id = ?`), id); err != nil {
		f.t.Fatal(err)
	}
	return m
}

func TestJobs_RunAtStartupThenEveryPassAndDailyInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.bulkTurns(1, "stuck-a", 25*time.Hour, false, "")
		f.seedFinishedWithoutModel("model-a")
		f.l.Start(t.Context())
		synctest.Wait()
		if n := f.count(`SELECT COUNT(*) FROM coordinator_turns WHERE id = 'stuck-a-0000' AND finished_at IS NOT NULL`); n != 1 {
			t.Fatal("startup did not run the daily settle")
		}
		if f.modelOf("model-a") != "m-1" {
			t.Fatal("startup did not run the pass")
		}

		f.seedFinishedWithoutModel("model-b")
		time.Sleep(passInterval - time.Second)
		synctest.Wait()
		if f.modelOf("model-b") != "" {
			t.Fatal("pass ran before its interval")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if f.modelOf("model-b") != "m-1" {
			t.Fatal("pass did not run after 10 minutes")
		}

		f.bulkTurns(1, "stuck-b", 25*time.Hour, false, "")
		time.Sleep(dailyInterval)
		synctest.Wait()
		if n := f.count(`SELECT COUNT(*) FROM coordinator_turns WHERE id = 'stuck-b-0000' AND finished_at IS NOT NULL`); n != 1 {
			t.Fatal("daily settle did not run after 24 hours")
		}
	})
}
