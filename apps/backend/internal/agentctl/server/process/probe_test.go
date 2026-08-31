package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestTurnStartMarker_IsZero covers turnStartMarker.isZero() across every
// platform's field combination — this file carries no build tag so it
// compiles and runs on every GOOS, unlike probe_linux_test.go /
// probe_darwin_test.go, which touch bootTicks/hasBootTicks (Linux) and
// wallTime (all platforms) respectively but only under their own tag.
func TestTurnStartMarker_IsZero(t *testing.T) {
	assert.True(t, turnStartMarker{}.isZero(), "the zero marker (no turn recorded) must report zero")

	linuxStyle := turnStartMarker{wallTime: time.Now(), bootTicks: 12345, hasBootTicks: true}
	assert.False(t, linuxStyle.isZero(), "a marker with a populated wallTime and boot ticks must not report zero")

	darwinStyle := turnStartMarker{wallTime: time.Now()}
	assert.False(t, darwinStyle.isZero(),
		"a marker with only wallTime populated (no boot ticks, e.g. Darwin) must not report zero")

	assert.True(t, turnStartMarker{bootTicks: 12345, hasBootTicks: true}.isZero(),
		"isZero must key off wallTime alone — bootTicks without a populated wallTime is never produced by newTurnStartMarker and must still read as zero")
}
