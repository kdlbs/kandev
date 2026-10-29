package pluginsdk

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestAutomationHistoryConsumerFixture compiles and runs the independent
// consumer module under testdata using only public SDK imports. A separate
// module is the evidence that the public package surface is usable outside
// this repository's own test packages.
func TestAutomationHistoryConsumerFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("consumer fixture compiles a separate module")
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain is unavailable")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "testdata", "pluginsdk", "automation-history-consumer"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, goBinary, "test", "-trimpath", "-race", "-count=1", "./...")
	command.Dir = fixture
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("independent consumer fixture failed: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Logf("consumer fixture output:\n%s", output)
	}
}
