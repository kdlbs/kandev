package agents

import (
	"testing"
)

// Fixtures follow upstream packages/coding-agent/src/cli/update-cli.ts at
// 2b562c5b7e3f and the installed omp/18.13.3 output.
func TestParseOmpUpdateCheck(t *testing.T) {
	const managed = "/opt/tern/bin/omp is installed and kept up to date by Tern; update it from Tern."
	unknown := func(reason NativeUpdateCheckReason, current, channel string) NativeUpdateCheckResult {
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUnknown, Reason: reason, CurrentVersion: current, Channel: channel}
	}
	cases := []struct {
		name   string
		stdout string
		stderr string
		exit   int
		want   NativeUpdateCheckResult
	}{
		{
			name:   "stable update available",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpdateAvailable, CurrentVersion: "18.13.3", LatestVersion: "18.14.0"},
		},
		{
			name:   "canary update available keeps prerelease latest",
			stdout: "Current version: 18.13.3\nCurrent channel: canary\nNew version available: 18.22.0-canary.3\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpdateAvailable, CurrentVersion: "18.13.3", LatestVersion: "18.22.0-canary.3", Channel: "canary"},
		},
		{
			name:   "up to date with icon and no channel",
			stdout: "Current version: 18.13.3\n✔ Already up to date\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpToDate, CurrentVersion: "18.13.3"},
		},
		{
			name:   "up to date without icon on canary",
			stdout: "Current version: 18.22.0-canary.3\r\nCurrent channel: canary\r\nAlready up to date\r\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpToDate, CurrentVersion: "18.22.0-canary.3", Channel: "canary"},
		},
		{
			name:   "ANSI colored output",
			stdout: "\x1b[2mCurrent version: 18.13.3\x1b[22m\n\x1b[2mCurrent channel: canary\x1b[22m\n\x1b[36mNew version available: 18.22.0-canary.3\x1b[39m\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpdateAvailable, CurrentVersion: "18.13.3", LatestVersion: "18.22.0-canary.3", Channel: "canary"},
		},
		{
			name:   "informational lines do not affect a recognized result",
			stdout: "Resolving release feed\nCurrent version: 18.13.3\n✔ Already up to date\n",
			want:   NativeUpdateCheckResult{State: NativeUpdateCheckStateUpToDate, CurrentVersion: "18.13.3"},
		},
		{
			name:   "installed failure without Error prefix",
			stdout: "Current version: 18.13.3\n",
			stderr: "Failed to check for updates: TypeError: ECONNREFUSED: Unable to connect. Is the computer able to access the url?\n",
			exit:   1,
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "18.13.3", ""),
		},
		{
			name:   "upstream failure with Error prefix",
			stdout: "Current version: 18.13.3\nCurrent channel: canary\n",
			stderr: "Error: Failed to check for updates: fetch failed\n",
			exit:   1,
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "18.13.3", "canary"),
		},
		{
			name:   "failure marker with zero exit",
			stdout: "Current version: 18.13.3\nFailed to check for updates: timeout\n",
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "18.13.3", ""),
		},
		{
			name:   "installation managed by another app",
			stdout: "\x1b[33m" + managed + "\x1b[39m\n",
			want:   unknown(NativeUpdateCheckReasonUnsupported, "", ""),
		},
		{
			name:   "conflicting outcome markers",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "repeated outcome marker",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0\nNew version available: 18.15.0\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "conflicting current versions",
			stdout: "Current version: 18.13.3\nCurrent version: 18.13.4\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "channel switch line",
			stdout: "Current version: 18.13.3\nSwitching to canary 18.22.0-canary.3\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "channel switch beside an available marker",
			stdout: "Current version: 18.13.3\nSwitching to stable 18.12.0 (downgrade from 18.13.3)\nNew version available: 18.14.0\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "forced reinstall line",
			stdout: "Current version: 18.13.3\nForcing reinstall of 18.13.3\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name: "empty output",
			want: unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "unrecognized text with zero exit",
			stdout: "Update check finished\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "available without current version",
			stdout: "New version available: 18.14.0\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "up to date without current version",
			stdout: "✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "invalid latest version",
			stdout: "Current version: 18.13.3\nNew version available: latest\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "invalid current version",
			stdout: "Current version: eighteen\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "build metadata is rejected",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0+sha.abc\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "invalid channel name",
			stdout: "Current version: 18.13.3\nCurrent channel: canary nightly\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "malformed latest line beside an up-to-date marker",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0 extra\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "malformed duplicate current version line",
			stdout: "Current version: 18.13.3\nCurrent version: invalid version\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "empty channel line",
			stdout: "Current version: 18.13.3\nCurrent channel:\n✔ Already up to date\n",
			want:   unknown(NativeUpdateCheckReasonUnrecognizedOutput, "", ""),
		},
		{
			name:   "non-zero exit never upgrades an available marker",
			stdout: "Current version: 18.13.3\nNew version available: 18.14.0\n",
			exit:   2,
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "18.13.3", ""),
		},
		{
			name:   "non-zero exit never upgrades an up-to-date marker",
			stdout: "Current version: 18.13.3\n✔ Already up to date\n",
			exit:   1,
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "18.13.3", ""),
		},
		{
			name:   "non-zero exit without any marker",
			stderr: "panic: unexpected\n",
			exit:   1,
			want:   unknown(NativeUpdateCheckReasonCheckFailed, "", ""),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOmpUpdateCheck(NativeUpdateCheckOutput{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: tc.exit})
			if got != tc.want {
				t.Errorf("ParseOmpUpdateCheck() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
