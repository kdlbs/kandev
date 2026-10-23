package hostmemory

import "testing"

func TestGatePressured(t *testing.T) {
	complete := Reading{Complete: true, TotalBytes: 64 << 30, AvailableBytes: 16 << 30, CommitUsedBytes: 50 << 30, CommitLimitBytes: 100 << 30, CommitKnown: true}
	cases := []struct {
		name      string
		gate      Gate
		reading   Reading
		pressured bool
		unknown   bool
	}{
		{
			name:    "disabled gate never pressures",
			gate:    Gate{Enabled: false, MinAvailableBytes: 32 << 30, MaxCommitPercent: 1},
			reading: complete,
		},
		{
			name:    "incomplete reading is unknown",
			gate:    Gate{Enabled: true, MinAvailableBytes: 1 << 30, MaxCommitPercent: 85},
			reading: Reading{Complete: false},
			unknown: true,
		},
		{
			name:      "available below floor",
			gate:      Gate{Enabled: true, MinAvailableBytes: 32 << 30, MaxCommitPercent: 0},
			reading:   complete,
			pressured: true,
		},
		{
			name:    "available at floor with low commit",
			gate:    Gate{Enabled: true, MinAvailableBytes: 16 << 30, MaxCommitPercent: 85},
			reading: complete,
		},
		{
			name:      "commit above ceiling",
			gate:      Gate{Enabled: true, MinAvailableBytes: 0, MaxCommitPercent: 40},
			reading:   complete,
			pressured: true,
		},
		{
			name:    "commit unknown is judged on available only",
			gate:    Gate{Enabled: true, MinAvailableBytes: 8 << 30, MaxCommitPercent: 40},
			reading: Reading{Complete: true, AvailableBytes: 16 << 30, CommitKnown: false},
		},
		{
			name:    "zero thresholds never pressure",
			gate:    Gate{Enabled: true, MinAvailableBytes: 0, MaxCommitPercent: 0},
			reading: Reading{Complete: true, AvailableBytes: 0, CommitUsedBytes: 100, CommitLimitBytes: 100, CommitKnown: true},
		},
		{
			name:      "commit limit zero is not judged",
			gate:      Gate{Enabled: true, MinAvailableBytes: 0, MaxCommitPercent: 1},
			reading:   Reading{Complete: true, CommitUsedBytes: 100, CommitLimitBytes: 0, CommitKnown: true},
			pressured: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pressured, unknown := tc.gate.Pressured(tc.reading)
			if pressured != tc.pressured || unknown != tc.unknown {
				t.Fatalf("Pressured() = (pressured=%v, unknown=%v), want (%v, %v)", pressured, unknown, tc.pressured, tc.unknown)
			}
		})
	}
}

func TestReadConfigDefaults(t *testing.T) {
	t.Setenv(EnvMinAvailableMB, "")
	t.Setenv(EnvMaxCommitPercent, "")
	// t.Setenv sets the variable to "", which is present; an empty threshold
	// keeps its default.
	gate := ReadConfig(false)
	if gate.Enabled {
		t.Fatal("Enabled = true, want the flag value false")
	}
	if gate.MinAvailableBytes != DefaultMinAvailableBytes {
		t.Fatalf("MinAvailableBytes = %d, want default %d", gate.MinAvailableBytes, DefaultMinAvailableBytes)
	}
	if gate.MaxCommitPercent != DefaultMaxCommitPercent {
		t.Fatalf("MaxCommitPercent = %v, want default %v", gate.MaxCommitPercent, DefaultMaxCommitPercent)
	}
}

func TestReadConfigOverrides(t *testing.T) {
	t.Setenv(EnvMinAvailableMB, "4096")
	t.Setenv(EnvMaxCommitPercent, "70")
	gate := ReadConfig(true)
	if !gate.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if gate.MinAvailableBytes != 4<<30 {
		t.Fatalf("MinAvailableBytes = %d, want %d", gate.MinAvailableBytes, uint64(4<<30))
	}
	if gate.MaxCommitPercent != 70 {
		t.Fatalf("MaxCommitPercent = %v, want 70", gate.MaxCommitPercent)
	}
}

func TestReadConfigIgnoresInvalidValues(t *testing.T) {
	t.Setenv(EnvMinAvailableMB, "not-a-number")
	t.Setenv(EnvMaxCommitPercent, "-3")
	gate := ReadConfig(true)
	if !gate.Enabled {
		t.Fatal("Enabled = false, want the flag value true")
	}
	if gate.MinAvailableBytes != DefaultMinAvailableBytes {
		t.Fatalf("MinAvailableBytes = %d, want default", gate.MinAvailableBytes)
	}
	if gate.MaxCommitPercent != DefaultMaxCommitPercent {
		t.Fatalf("MaxCommitPercent = %v, want default", gate.MaxCommitPercent)
	}
}

const procMeminfoOvercommitted = `MemTotal:       16384000 kB
MemFree:         1000000 kB
MemAvailable:   12000000 kB
Buffers:          100000 kB
Cached:          2000000 kB
CommitLimit:     8192000 kB
Committed_AS:   20000000 kB
`

// Under the heuristic and always overcommit modes the kernel does not enforce
// CommitLimit, so a Committed_AS far above it is normal and must not pressure.
func TestReadingFromProcIgnoresCommitUnlessOvercommitStrict(t *testing.T) {
	gate := Gate{Enabled: true, MinAvailableBytes: 8 << 30, MaxCommitPercent: 75}
	for _, mode := range []string{"0\n", "1\n", ""} {
		reading, err := readingFromProc(procMeminfoOvercommitted, mode)
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if reading.CommitKnown {
			t.Fatalf("mode %q: commit must not be judged", mode)
		}
		if pressured, unknown := gate.Pressured(reading); pressured || unknown {
			t.Fatalf("mode %q: pressured=%v unknown=%v, want a clear host", mode, pressured, unknown)
		}
	}
	reading, err := readingFromProc(procMeminfoOvercommitted, "2\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reading.CommitKnown || reading.CommitLimitBytes != 8192000<<10 || reading.CommitUsedBytes != 20000000<<10 {
		t.Fatalf("strict mode reading = %+v, want the commit pair", reading)
	}
	if pressured, _ := gate.Pressured(reading); !pressured {
		t.Fatal("strict overcommit above the commit ceiling must pressure")
	}
}

func TestReadingFromProcAvailableFallbackAndMissingTotal(t *testing.T) {
	reading, err := readingFromProc("MemTotal: 1000 kB\nMemFree: 10 kB\nBuffers: 20 kB\nCached: 30 kB\n", "0")
	if err != nil {
		t.Fatal(err)
	}
	if !reading.Complete || reading.TotalBytes != 1000<<10 || reading.AvailableBytes != 60<<10 {
		t.Fatalf("fallback reading = %+v", reading)
	}
	if reading, err := readingFromProc("MemFree: 10 kB\n", "2"); err == nil || reading.Complete {
		t.Fatalf("missing MemTotal must be an incomplete reading, got %+v err=%v", reading, err)
	}
}
