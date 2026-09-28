package launcher

import (
	"bufio"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// agentctl writes its own structured logs to stdout, not stderr. Relaying
// stdout unconditionally at DEBUG means the child's INFO records never survive
// the parent's default INFO file level, so its permission decisions are
// recorded nowhere. AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.9.
func TestPipeOutputPreservesChildLevelOnStdout(t *testing.T) {
	tests := []struct {
		name string
		line string
		want zapcore.Level
	}{
		{
			name: "child info survives",
			line: "2026-09-22T12:00:00.000Z\tINFO\tprocess/manager.go:2756\thandling permission request",
			want: zapcore.InfoLevel,
		},
		{
			name: "child warn survives",
			line: "2026-09-22T12:00:00.000Z\tWARN\tacp/client.go:141\tpermission request carries no options",
			want: zapcore.WarnLevel,
		},
		{
			name: "child error survives",
			line: "2026-09-22T12:00:00.000Z\tERROR\tprocess/manager.go:2920\tfailed to deliver permission notification",
			want: zapcore.ErrorLevel,
		},
		{
			name: "child debug stays debug",
			line: "2026-09-22T12:00:00.000Z\tDEBUG\tprocess/manager.go:2586\tagent stderr",
			want: zapcore.DebugLevel,
		},
		{
			name: "unstructured stdout stays debug",
			line: "some unstructured passthrough noise",
			want: zapcore.DebugLevel,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("NewFromZap returned error: %v", err)
			}
			l := &Launcher{logger: log}

			l.pipeOutput("stdout", bufio.NewScanner(strings.NewReader(test.line)))

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("relayed %d entries, want 1", len(entries))
			}
			if entries[0].Level != test.want {
				t.Fatalf("relayed at %s, want %s", entries[0].Level, test.want)
			}
		})
	}
}
