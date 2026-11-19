package agents

import "testing"

// resolvePathAgainst mirrors Node's path.resolve(cwd, entry) used by OMP for
// PI_CONFIG_FILES and npm_config_userconfig, including Windows root-relative
// and drive-relative forms.
func TestResolvePathAgainstWindowsSemantics(t *testing.T) {
	cwd := `D:\work\repo`
	for _, tc := range []struct{ entry, want string }{
		{`C:\abs\channel.yml`, `C:\abs\channel.yml`},
		{`C:/abs/channel.yml`, `C:/abs/channel.yml`},
		{`\\server\share\channel.yml`, `\\server\share\channel.yml`},
		{`\omp-config\channel.yml`, `D:\omp-config\channel.yml`},
		{`/omp-config/channel.yml`, `D:/omp-config/channel.yml`},
		{`D:rel\channel.yml`, `D:\work\repo\rel\channel.yml`},
		{`E:rel\channel.yml`, `E:\rel\channel.yml`},
		{`rel\channel.yml`, `D:\work\repo\rel\channel.yml`},
	} {
		if got := resolvePathAgainst(cwd, tc.entry, true); got != tc.want {
			t.Errorf("resolvePathAgainst(%q) = %q, want %q", tc.entry, got, tc.want)
		}
	}
}
