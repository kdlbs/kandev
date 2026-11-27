package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The full reader/writer and forward-upgrade matrix runs through browser
// fixtures; this gate prevents substituting a rebuilt artifact for frozen N.
func TestDeliveryRetentionCompatibility(t *testing.T) {
	binary := os.Getenv("KANDEV_RETENTION_COMPAT_BINARY")
	if binary == "" {
		t.Skip("set KANDEV_RETENTION_COMPAT_BINARY to run the frozen artifact gate")
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != "3df1e310220f0a7588453c062f2fa90aa0a611dd7277cc77c76dd4f8bc3a0517" {
		t.Fatalf("frozen Release N artifact changed: %s", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("Release N version: %s %v", out, err)
	}
	if !strings.Contains(string(out), "v0.97.0-143-gd7a44e96ed-dirty") {
		t.Fatalf("unexpected Release N identity: %s", out)
	}
}
