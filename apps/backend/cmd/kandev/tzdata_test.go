package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

const tzdataHelperEnv = "KANDEV_TZDATA_HELPER"

// The backend binary embeds the IANA timezone database (time/tzdata) so that
// LoadLocation resolves an IANA zone with no system zoneinfo and no usable
// GOROOT. runtime.GOROOT reads the environment captured at process start, so
// the probe runs in a child process whose GOROOT points at an empty directory
// — a deployed Windows binary, whose build-time GOROOT does not exist on the
// user's machine. On macOS and Linux a system zoneinfo makes the child pass
// either way; Windows is where the missing import fails. The probe also
// computes a routine's next fire time from a named zone, so the scheduler
// path is covered, not only the raw LoadLocation call.
func TestEmbeddedTZDataLoadsIANAZoneWithoutSystemZoneinfo(t *testing.T) {
	if os.Getenv(tzdataHelperEnv) == "1" {
		if _, err := time.LoadLocation("Asia/Seoul"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// The routine scheduler resolves a trigger's timezone through
		// NextCronTime, so exercise that path too: a schedule in a named IANA
		// zone must compute without any system zoneinfo.
		next, err := shared.NextCronTime("0 9 * * *", "Asia/Seoul", time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if next.IsZero() {
			fmt.Fprintln(os.Stderr, "NextCronTime returned the zero time")
			os.Exit(1)
		}
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=TestEmbeddedTZDataLoadsIANAZoneWithoutSystemZoneinfo")
	cmd.Env = append(os.Environ(), tzdataHelperEnv+"=1", "GOROOT="+t.TempDir(), "ZONEINFO=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("LoadLocation(Asia/Seoul) failed in a process without a usable GOROOT: %v\n%s", err, out)
	}
}
