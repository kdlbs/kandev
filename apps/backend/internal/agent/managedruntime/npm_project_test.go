package managedruntime

import (
	"runtime"
	"testing"
)

func TestHomeForNPMUsesPlatformSpecificEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		home        string
		userProfile string
		want        string
	}{
		{
			name:        "windows uses user profile",
			goos:        "windows",
			home:        `C:\Git\home`,
			userProfile: `C:\Users\agent`,
			want:        `C:\Users\agent`,
		},
		{
			name:        "unix uses home",
			goos:        "linux",
			home:        "/home/agent",
			userProfile: `C:\Users\agent`,
			want:        "/home/agent",
		},
		{
			name:        "empty preferred value falls back",
			goos:        "windows",
			home:        "/home/agent",
			userProfile: "  ",
			want:        "/home/agent",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := homeForNPM(test.goos, test.home, test.userProfile); got != test.want {
				t.Fatalf("homeForNPM() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNPMProjectHomeSourcesUseSamePlatformPreference(t *testing.T) {
	const (
		home        = "/posix/home"
		userProfile = `C:\Users\agent`
	)
	want := homeForNPM(runtime.GOOS, home, userProfile)
	if got := homeFromMap(map[string]string{"HOME": home, "USERPROFILE": userProfile}); got != want {
		t.Fatalf("homeFromMap() = %q, want %q", got, want)
	}
	env := []string{"HOME=" + home, "USERPROFILE=" + userProfile}
	if got := homeFromEnvironment(env); got != want {
		t.Fatalf("homeFromEnvironment() = %q, want %q", got, want)
	}
}
