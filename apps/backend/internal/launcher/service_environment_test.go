package launcher

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRenderSystemdUnitPreservesEnvironmentLiterals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native service paths are POSIX-only")
	}
	fixtures := []struct {
		name    string
		path    string
		encoded string
		quote   string
	}{
		{name: "plain", path: "plain", encoded: "plain"},
		{name: "single dollar", path: "home$literal", encoded: "home$literal", quote: `"`},
		{name: "adjacent dollars", path: "home$$literal", encoded: "home$$literal", quote: `"`},
		{name: "variable shaped", path: "$USER", encoded: "$USER", quote: `"`},
		{name: "braced variable", path: "${HOME}", encoded: "${HOME}", quote: `"`},
		{name: "trailing dollar", path: "home$", encoded: "home$", quote: `"`},
		{name: "ordinary characters", path: `home "quoted"\dir%h`, encoded: `home \"quoted\"\\dir%%h`, quote: `"`},
	}
	for _, system := range []bool{false, true} {
		for _, fromConfig := range []bool{false, true} {
			for _, fixture := range fixtures {
				t.Run(fmt.Sprintf("system=%t/config-home=%t/%s", system, fromConfig, fixture.name), func(t *testing.T) {
					input := nativeServiceUnitInput{
						Executable:        "/opt/kandev/bin/kandev",
						HomeDir:           "/srv/" + fixture.path,
						ConfigFile:        "/srv/" + fixture.path + "/selected.yaml",
						BundleDir:         "/srv/" + fixture.path + "/bundle",
						System:            system,
						HomeDirFromConfig: fromConfig,
					}
					unit := renderSystemdUnit(input)
					prefix, suffix := "Environment="+fixture.quote, fixture.quote
					assertServiceRenderedLines(t, unit,
						prefix+"KANDEV_INTERNAL_CONFIG_FILE=/srv/"+fixture.encoded+"/selected.yaml"+suffix,
						prefix+"KANDEV_SERVICE_METADATA=/srv/"+fixture.encoded+"/service/install.json"+suffix,
						prefix+"KANDEV_BUNDLE_DIR=/srv/"+fixture.encoded+"/bundle"+suffix,
					)
					if fromConfig {
						if strings.Contains(unit, "KANDEV_HOME_DIR=") {
							t.Error("configuration-owned home must not become an environment override")
						}
					} else {
						assertServiceRenderedLines(t, unit, prefix+"KANDEV_HOME_DIR=/srv/"+fixture.encoded+suffix)
					}
				})
			}
		}
	}
}

func TestRenderSystemdUnitPreservesPathLiteralsAndSpecifiers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native service paths are POSIX-only")
	}
	for _, system := range []bool{false, true} {
		unit := renderSystemdUnit(nativeServiceUnitInput{
			Executable:   "/opt/kandev/bin/kandev",
			HomeDir:      "/srv/kandev",
			System:       system,
			PathPrefixes: []string{`/opt/node$stable/bin`, `/opt/node with "quotes"\bin`, `/opt/untrusted%h/bin`},
		})
		assertServiceRenderedLines(t, unit,
			`Environment="PATH=/opt/node$stable/bin:/opt/node with \"quotes\"\\bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:%h/.local/bin:%h/.bun/bin:%h/.opencode/bin:%h/.npm-global/bin"`,
		)
		if strings.Contains(unit, "/opt/untrusted") {
			t.Error("percent-bearing PATH prefix must remain ineligible")
		}
	}
}

func TestRenderSystemdUnitKeepsCommandQuoting(t *testing.T) {
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: `/opt/a $USER %h/"quoted"\bin/kandev`,
		HomeDir:    "/srv/kandev",
	})
	assertServiceRenderedLines(t, unit,
		`ExecStart="/opt/a $$USER %%h/\"quoted\"\\bin/kandev" --headless`,
	)
}

func TestRenderLaunchdPlistPreservesEnvironmentLiterals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native service paths are POSIX-only")
	}
	for _, system := range []bool{false, true} {
		plist := renderLaunchdPlist(nativeServiceUnitInput{
			Executable:   `/opt/$USER %h/"<&>"/kandev`,
			HomeDir:      `/srv/home$literal %h "<&>"\dir`,
			ConfigFile:   `/etc/config$literal.yaml`,
			BundleDir:    `/opt/bundle$literal`,
			PathPrefixes: []string{`/opt/node$stable/bin`},
			System:       system,
		})
		assertServiceRenderedLines(t, plist,
			`    <string>/opt/$USER %h/&quot;&lt;&amp;&gt;&quot;/kandev</string>`,
			`    <string>--headless</string>`,
			`      <string>/srv/home$literal %h &quot;&lt;&amp;&gt;&quot;\dir</string>`,
			`      <string>/srv/home$literal %h &quot;&lt;&amp;&gt;&quot;\dir/service/install.json</string>`,
			`      <string>/etc/config$literal.yaml</string>`,
			`      <string>/opt/bundle$literal</string>`,
			`      <string>/opt/node$stable/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>`,
		)
	}
}

func assertServiceRenderedLines(t *testing.T, rendered string, wantLines ...string) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	for _, want := range wantLines {
		if !slices.Contains(lines, want) {
			t.Errorf("rendered service missing exact line %q:\n%s", want, rendered)
		}
	}
}
