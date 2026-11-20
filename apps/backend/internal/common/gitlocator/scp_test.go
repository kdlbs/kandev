package gitlocator

import (
	"errors"
	"strings"
	"testing"
)

func TestParseConventionalSCP(t *testing.T) {
	for _, test := range []struct {
		name    string
		locator string
		host    string
		path    string
	}{
		{name: "github", locator: "git@github.com:org/repo.git", host: "github.com", path: "org/repo.git"},
		{name: "gitlab nested", locator: "git@gitlab.com:group/subgroup/project.git", host: "gitlab.com", path: "group/subgroup/project.git"},
		{name: "self managed", locator: "git@git.internal:team/repo", host: "git.internal", path: "team/repo"},
		{name: "single segment", locator: "git@github.com:repository", host: "github.com", path: "repository"},
		{name: "single label host", locator: "git@localhost:team/repo", host: "localhost", path: "team/repo"},
		{name: "ipv4 host", locator: "git@192.168.1.10:team/repo", host: "192.168.1.10", path: "team/repo"},
		{name: "letter t", locator: "git@git.internal:team/Test_tool-1.2", host: "git.internal", path: "team/Test_tool-1.2"},
		{name: "hyphenated labels", locator: "git@git-1.internal-host:org/repo", host: "git-1.internal-host", path: "org/repo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := ParseConventionalSCP(test.locator)
			if err != nil {
				t.Fatalf("ParseConventionalSCP(%q) returned %v", test.locator, err)
			}
			if parsed.Host != test.host || parsed.Path != test.path {
				t.Fatalf("parsed = %#v, want host %q path %q", parsed, test.host, test.path)
			}
		})
	}
}

func TestParseConventionalSCPRejections(t *testing.T) {
	longLabel := strings.Repeat("a", 64)
	longHost := strings.Repeat(longLabel[:63]+".", 4) + "com"
	for _, locator := range []string{
		"",
		"git@",
		"git@github.com",
		"git@github.com:",
		"git@:org/repo",
		"git@github.com:org/",
		"git@github.com:/org/repo",
		"git@github.com:org//repo",
		"git@github.com:org/./repo",
		"git@github.com:org/../repo",
		"git@github.com:org/-repo",
		"git@github.com:org/repo.git/",
		"git@github.com:org/re po",
		"git@github.com:org/re\tpo",
		"git@github.com:org/re\npo",
		"git@github.com:org/re@po",
		"git@github.com:org/re:po",
		"git@github.com:org/re?po",
		"git@github.com:org/re#po",
		"git@github.com:org/re%20po",
		"git@github.com:org/re;po",
		"git@github.com:org/re|po",
		"git@github.com:org/re\\po",
		"git@github.com:org/~repo",
		"git@github.com:~org/repo",
		"git@github.com:org/repo ",
		" git@github.com:org/repo",
		"git@github.com:org/repo.lock.git.lock/../repo",
		"ssh@github.com:org/repo",
		"git@github.com:22:org/repo",
		"git@[2001:db8::1]:org/repo",
		"git@github.com..:org/repo",
		"git@.github.com:org/repo",
		"git@github.com.:org/repo",
		"git@github..com:org/repo",
		"git@-github.com:org/repo",
		"git@github-.com:org/repo",
		"git@github_.com:org/repo",
		"git@git hub.com:org/repo",
		"git@höst.com:org/repo",
		"git@999.999.999.999:org/repo",
		"git@1.2.3:org/repo",
		"git@" + longLabel + ":org/repo",
		"git@" + longHost + ":org/repo",
		"https://github.com/org/repo.git",
		"ssh://git@github.com/org/repo.git",
		"git@github.com:org/repo.git?ref=main",
		"git@github.com:org/repo.git#main",
	} {
		t.Run(locator, func(t *testing.T) {
			_, err := ParseConventionalSCP(locator)
			if !errors.Is(err, ErrInvalidConventionalSCP) {
				t.Fatalf("ParseConventionalSCP(%q) error = %v, want ErrInvalidConventionalSCP", locator, err)
			}
		})
	}
}
