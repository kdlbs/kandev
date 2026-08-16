package websocket

import (
	"os"
	"strings"
	"testing"
)

// TestDumpShimProbe writes the REAL runtime shim (with the probe export
// injected) to /tmp/shim_probe.js so the node validator can exercise the
// actual functions r/rn/sc/nz/norm/rwaStyle/srcsetParts/mref exactly as
// shipped, including the observer-loop idempotency behavior.
//
// It is a diagnostic, not a behavioral test (Go cannot execute the shim's
// JS), so it is skipped unless KANDEV_DUMP_SHIM=1 is set — normal test runs
// never write the /tmp artifact. Invocation:
//
//	KANDEV_DUMP_SHIM=1 go test -run TestDumpShimProbe ./internal/gateway/websocket/
//	node /tmp/shim_probe_validate.js
func TestDumpShimProbe(t *testing.T) {
	if os.Getenv("KANDEV_DUMP_SHIM") == "" {
		t.Skip("diagnostic only: set KANDEV_DUMP_SHIM=1 to dump the shim for the node validator")
	}
	shim := runtimeShim(proxyPrefix, "cap.ABC-_123")
	idx := strings.LastIndex(shim, "})();")
	if idx < 0 {
		t.Fatal("shim template does not end with the IIFE close")
	}
	probe := shim[:idx] + `window.__kandev__={r:r,rn:rn,sc:sc,nz:nz,norm:norm,rwaStyle:rwaStyle,srcsetParts:srcsetParts,mref:mref,rwa:rwa};` + shim[idx:]
	if err := os.WriteFile("/tmp/shim_probe.js", []byte(probe), 0o644); err != nil {
		t.Fatalf("write probe: %v", err)
	}
}
