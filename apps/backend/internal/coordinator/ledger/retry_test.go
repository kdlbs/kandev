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
