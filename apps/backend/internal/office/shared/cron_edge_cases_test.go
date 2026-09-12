package shared

import (
	"testing"
	"time"
)

func TestNextCronTime_PastLeapYearPOSIXExtensionTerminates(t *testing.T) {
	const expression = "0 12 1 1 *"
	after := time.Date(2040, time.January, 2, 0, 0, 0, 0, time.UTC)
	want := time.Date(2041, time.January, 1, 17, 0, 0, 0, time.UTC)

	done := make(chan struct{})
	var got time.Time
	var err error
	go func() {
		got, err = NextCronTime(expression, "America/New_York", after)
		close(done)
	}()

	select {
	case <-done:
		if err != nil {
			t.Fatalf("NextCronTime returned error: %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("NextCronTime = %s, want %s", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("NextCronTime did not terminate across a POSIX timezone year boundary")
	}
}
