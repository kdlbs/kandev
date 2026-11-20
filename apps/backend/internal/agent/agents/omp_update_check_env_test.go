package agents

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestOmpUpdateCheckFoldedEnvMatchesUpstream(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "selected")
	other := filepath.Join(root, "other")
	cases := []struct {
		name string
		env  []string
	}{
		{"empty exact key must not mask nonempty folded value", []string{"XDG_CONFIG_HOME=", "xdg_config_home=" + selected}},
		{"first nonempty folded value wins", []string{"xdg_config_home=" + selected, "Xdg_Config_Home=" + other}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewNativeUpdateCheckResolver(tc.env, root, "")
			files := NewOmpACP().NativeUpdateCheck().Context.Sources(r).Files
			want := filepath.Join(selected, ".bunfig.toml")
			if !slices.Contains(files, want) {
				t.Fatalf("upstream-selected bunfig %s is not fingerprinted; candidates=%v", want, files)
			}
		})
	}
}
