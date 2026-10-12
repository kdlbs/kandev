package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestManagerConfigureManagedGitTools(t *testing.T) {
	for _, replaceIndexed := range []bool{false, true} {
		mode := "overlay"
		if replaceIndexed {
			mode = "complete"
		}
		t.Run(mode, func(t *testing.T) {
			shimDir := filepath.Join(t.TempDir(), "installed github shims")
			startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
			parentEnv := filepath.Join(t.TempDir(), "user bash env.sh")
			initial := map[string]string{
				"PATH":                             filepath.Join("/usr", "bin"),
				"BASH_ENV":                         parentEnv,
				"GH_TOKEN":                         "profile-token",
				githubauth.CredentialHelperPathEnv: "/installed/agentctl",
				githubauth.CredentialCLIShimDirEnv: shimDir,
				githubauth.CredentialCLIBashEnvEnv: startupEnv,
				"GIT_CONFIG_COUNT":                 "1",
				"GIT_CONFIG_KEY_0":                 "core.hooksPath",
				"GIT_CONFIG_VALUE_0":               "/user/hooks",
			}
			if replaceIndexed {
				initial[githubauth.CredentialBrokerURLEnv] = "https://old-broker.example/resolve"
				initial[githubauth.CredentialLeaseEnv] = "old-lease"
				initial[githubauth.CredentialRepositoryEnv] = "old-repository"
				initial[githubauth.CredentialOwnerEnv] = "old-owner"
				initial[githubauth.CredentialParentBashEnv] = parentEnv
				initial["PATH"] = shimDir + string(filepath.ListSeparator) + initial["PATH"]
				initial["BASH_ENV"] = startupEnv
			}

			cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
			for key, value := range initial {
				cfg.AgentEnv = append(cfg.AgentEnv, key+"="+value)
			}
			mgr := NewManager(cfg, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })

			configure := mgr.Configure
			if replaceIndexed {
				configure = mgr.ConfigureWithEnvironment
			}
			request := map[string]string{
				githubauth.CredentialBrokerURLEnv:  "https://current-broker.example/resolve",
				githubauth.CredentialLeaseEnv:      "current-lease",
				githubauth.CredentialRepositoryEnv: "current-repository",
				githubauth.CredentialOwnerEnv:      "current-owner",
				githubauth.CredentialTaskIDEnv:     "current-task",
				githubauth.CredentialSessionIDEnv:  "current-session",
				"GIT_CONFIG_COUNT":                 "1",
				"GIT_CONFIG_KEY_0":                 "core.hooksPath",
				"GIT_CONFIG_VALUE_0":               "/user/hooks",
			}
			require.NoError(t, configure("echo", nil, false, request, "", nil, false))

			require.Equal(t, "https://current-broker.example/resolve", envValue(cfg.AgentEnv, githubauth.CredentialBrokerURLEnv))
			require.Equal(t, "current-lease", envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
			require.Equal(t, "current-repository", envValue(cfg.AgentEnv, githubauth.CredentialRepositoryEnv))
			require.Equal(t, "/installed/agentctl", envValue(cfg.AgentEnv, githubauth.CredentialHelperPathEnv))
			require.Equal(t, shimDir, envValue(cfg.AgentEnv, githubauth.CredentialCLIShimDirEnv))
			require.Equal(t, startupEnv, envValue(cfg.AgentEnv, githubauth.CredentialCLIBashEnvEnv))
			if runtime.GOOS == "windows" {
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, initial["BASH_ENV"], envValue(cfg.AgentEnv, "BASH_ENV"))
			} else {
				require.Equal(t, parentEnv, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, startupEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
			}
			require.Equal(t, shimDir+string(filepath.ListSeparator)+initialPath(initial, shimDir), envValue(cfg.AgentEnv, "PATH"))
			require.Equal(t, "profile-token", envValue(cfg.AgentEnv, "GH_TOKEN"))
			require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "old-lease")
			require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "old-repository")
		})
	}
}

func initialPath(initial map[string]string, shimDir string) string {
	path := initial["PATH"]
	if strings.HasPrefix(path, shimDir+string(filepath.ListSeparator)) {
		return strings.TrimPrefix(path, shimDir+string(filepath.ListSeparator))
	}
	return path
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestManagerConfigureManagedGitToolsDeactivateAndReactivate(t *testing.T) {
	for _, replaceIndexed := range []bool{false, true} {
		mode := "overlay"
		if replaceIndexed {
			mode = "complete"
		}
		for _, replacement := range []string{"nil", "empty", "partial"} {
			t.Run(mode+"/"+replacement, func(t *testing.T) {
				shimDir := filepath.Join(t.TempDir(), "installed shims")
				startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
				parentEnv := filepath.Join(t.TempDir(), "parent.sh")
				initial := map[string]string{
					"PATH":                             shimDir + string(filepath.ListSeparator) + "/usr/bin",
					"BASH_ENV":                         startupEnv,
					githubauth.CredentialBrokerURLEnv:  "https://old-broker.example/resolve",
					githubauth.CredentialLeaseEnv:      "old-lease",
					githubauth.CredentialHelperPathEnv: "/installed/agentctl",
					githubauth.CredentialCLIShimDirEnv: shimDir,
					githubauth.CredentialCLIBashEnvEnv: startupEnv,
					githubauth.CredentialParentBashEnv: parentEnv,
				}
				cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
				for key, value := range initial {
					cfg.AgentEnv = append(cfg.AgentEnv, key+"="+value)
				}
				mgr := NewManager(cfg, newTestLogger(t))
				t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
				configure := mgr.Configure
				if replaceIndexed {
					configure = mgr.ConfigureWithEnvironment
				}

				require.NoError(t, configure("echo", nil, false, managedGitToolContract("current-lease"), "", nil, false))
				var withoutManaged map[string]string
				switch replacement {
				case "empty":
					withoutManaged = map[string]string{}
				case "partial":
					withoutManaged = map[string]string{"CURRENT_SETTING": "preserved"}
				}
				require.NoError(t, configure("echo", nil, false, withoutManaged, "", nil, false))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialBrokerURLEnv))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialHelperPathEnv))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialCLIShimDirEnv))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialCLIBashEnvEnv))
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, "/usr/bin", envValue(cfg.AgentEnv, "PATH"))
				if runtime.GOOS == "windows" {
					require.Equal(t, startupEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
				} else {
					require.Equal(t, parentEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
				}
				require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), githubauth.ManagedGitCredentialHelper)
				if replacement == "partial" {
					require.Equal(t, "preserved", envValue(cfg.AgentEnv, "CURRENT_SETTING"))
				}

				require.NoError(t, configure("echo", nil, false, managedGitToolContract("new-lease"), "", nil, false))
				require.Equal(t, "new-lease", envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
				require.Equal(t, "/installed/agentctl", envValue(cfg.AgentEnv, githubauth.CredentialHelperPathEnv))
				require.Equal(t, shimDir, envValue(cfg.AgentEnv, githubauth.CredentialCLIShimDirEnv))
				require.Equal(t, shimDir+string(filepath.ListSeparator)+"/usr/bin", envValue(cfg.AgentEnv, "PATH"))
				require.Equal(t, filepath.Join(shimDir, githubauth.CLIBashEnvFilename), envValue(cfg.AgentEnv, "BASH_ENV"))
			})
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestManagerConfiguredManagedGitToolsSnapshotRoundTrip(t *testing.T) {
	for _, replaceIndexed := range []bool{false, true} {
		mode := "overlay"
		if replaceIndexed {
			mode = "complete"
		}
		t.Run(mode, func(t *testing.T) {
			shimDir := filepath.Join(t.TempDir(), "installed shims")
			startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
			parentEnv := filepath.Join(t.TempDir(), "user hook.sh")
			parentMarker := filepath.Join(t.TempDir(), "parent-hook-ran")
			require.NoError(t, os.WriteFile(parentEnv, []byte("printf ran > \"$KANDEV_TEST_PARENT_MARKER\"\n"), 0o600))
			require.NoError(t, os.MkdirAll(shimDir, 0o700))
			require.NoError(t, os.WriteFile(startupEnv, []byte("if [ -n \"${KANDEV_GITHUB_PARENT_BASH_ENV:-}\" ]; then . \"$KANDEV_GITHUB_PARENT_BASH_ENV\"; fi\n"), 0o600))

			cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{
				"PATH=/usr/bin" + string(filepath.ListSeparator) + "/bin",
				"BASH_ENV=" + parentEnv,
				"KANDEV_TEST_PARENT_MARKER=" + parentMarker,
				githubauth.CredentialHelperPathEnv + "=/installed/agentctl",
				githubauth.CredentialCLIShimDirEnv + "=" + shimDir,
				githubauth.CredentialCLIBashEnvEnv + "=" + startupEnv,
			}}
			mgr := NewManager(cfg, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
			configure := mgr.Configure
			if replaceIndexed {
				configure = mgr.ConfigureWithEnvironment
			}
			require.NoError(t, configure("echo", nil, false, managedGitToolContract("round-trip-lease"), "", nil, false))

			// A lifecycle handoff can send the full effective AgentEnv back as a new snapshot.
			snapshot := environmentMapFromSlice(cfg.AgentEnv)
			if runtime.GOOS == "windows" {
				require.Equal(t, parentEnv, snapshot["BASH_ENV"])
				require.Empty(t, snapshot[githubauth.CredentialParentBashEnv])
			} else {
				require.Equal(t, startupEnv, snapshot["BASH_ENV"])
				require.Equal(t, parentEnv, snapshot[githubauth.CredentialParentBashEnv])
			}
			require.NoError(t, configure("echo", nil, false, snapshot, "", nil, false))

			if runtime.GOOS == "windows" {
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, parentEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
			} else {
				require.Equal(t, parentEnv, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, startupEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
				bash, err := exec.LookPath("bash")
				require.NoError(t, err)
				cmd := exec.Command(bash, "-c", ":")
				cmd.Env = append([]string(nil), cfg.AgentEnv...)
				require.NoError(t, cmd.Run())
				require.FileExists(t, parentMarker, "the user hook must run through the installed wrapper")
			}

			require.NoError(t, configure("echo", nil, false, nil, "", nil, false))
			require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialBrokerURLEnv))
			require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
			if runtime.GOOS == "windows" {
				require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialParentBashEnv))
				require.Equal(t, parentEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
			} else {
				require.Equal(t, parentEnv, envValue(cfg.AgentEnv, "BASH_ENV"))
				if err := os.Remove(parentMarker); err != nil && !os.IsNotExist(err) {
					require.NoError(t, err)
				}
				bash, err := exec.LookPath("bash")
				require.NoError(t, err)
				cmd := exec.Command(bash, "-c", ":")
				cmd.Env = append([]string(nil), cfg.AgentEnv...)
				require.NoError(t, cmd.Run())
				require.FileExists(t, parentMarker, "the unmanaged environment must restore and run the user hook")
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestManagerConfiguredManagedGitToolsSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX Git helpers and Bash")
	}
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)

	root := t.TempDir()
	shimDir := filepath.Join(root, "installed github shims")
	require.NoError(t, os.MkdirAll(shimDir, 0o700))
	helper := filepath.Join(root, "agentctl")
	helperScript := `#!/bin/sh
[ "$1" = git-credential ] || exit 1
[ "$KANDEV_GITHUB_CREDENTIAL_LEASE" = current-lease ] || exit 1
allowed=
while IFS= read -r line; do
  case "$line" in path=owner/allowed.git) allowed=yes;; esac
done
[ "$allowed" = yes ] || exit 1
printf 'username=synthetic\npassword=synthetic\n'
`
	require.NoError(t, os.WriteFile(helper, []byte(helperScript), 0o700))
	ghShim := filepath.Join(shimDir, "gh")
	require.NoError(t, os.WriteFile(ghShim, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	parentEnv := filepath.Join(root, "parent bash env.sh")
	parentMarker := filepath.Join(root, "parent-hook-ran")
	require.NoError(t, os.WriteFile(parentEnv, []byte("#!/bin/sh\nprintf ran > \"$KANDEV_TEST_PARENT_MARKER\"\n"), 0o700))
	startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
	startupScript := "#!/bin/sh\n" +
		"if [ -n \"${KANDEV_GITHUB_PARENT_BASH_ENV:-}\" ]; then . \"$KANDEV_GITHUB_PARENT_BASH_ENV\"; fi\n" +
		"case \":${PATH:-}:\" in *:\"${KANDEV_GITHUB_CLI_SHIM_DIR}\":*) ;; *) PATH=\"${KANDEV_GITHUB_CLI_SHIM_DIR}:${PATH:-}\"; export PATH ;; esac\n"
	require.NoError(t, os.WriteFile(startupEnv, []byte(startupScript), 0o700))
	ambientHelper := filepath.Join(root, "ambient-helper")
	ambientMarker := filepath.Join(root, "ambient-helper-ran")
	ambientScript := "#!/bin/sh\nprintf ran > " + ambientMarker + "\nexit 0\n"
	require.NoError(t, os.WriteFile(ambientHelper, []byte(ambientScript), 0o700))

	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + root,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"KANDEV_TEST_PARENT_MARKER=" + parentMarker,
		"BASH_ENV=" + parentEnv,
		githubauth.CredentialBrokerURLEnv + "=https://old-broker.example/resolve",
		githubauth.CredentialLeaseEnv + "=old-lease",
		githubauth.CredentialRepositoryEnv + "=old-repository",
		githubauth.CredentialHelperPathEnv + "=" + helper,
		githubauth.CredentialCLIShimDirEnv + "=" + shimDir,
		githubauth.CredentialCLIBashEnvEnv + "=" + startupEnv,
	}}
	mgr := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	request := managedGitToolContract("current-lease")
	request["PATH"] = "/usr/bin:/bin"
	request["HOME"] = root
	request["GIT_CONFIG_NOSYSTEM"] = "1"
	request["GIT_CONFIG_GLOBAL"] = os.DevNull
	request["GIT_TERMINAL_PROMPT"] = "0"
	request["GIT_ASKPASS"] = ""
	request["SSH_ASKPASS"] = ""
	request["KANDEV_TEST_PARENT_MARKER"] = parentMarker
	request["BASH_ENV"] = parentEnv
	request["GIT_CONFIG_COUNT"] = "4"
	request["GIT_CONFIG_KEY_0"] = "credential.https://github.com.helper"
	request["GIT_CONFIG_VALUE_0"] = "!" + ambientHelper
	request["GIT_CONFIG_KEY_1"] = "credential.https://github.com.helper"
	request["GIT_CONFIG_VALUE_1"] = ""
	request["GIT_CONFIG_KEY_2"] = "credential.https://github.com.helper"
	request["GIT_CONFIG_VALUE_2"] = githubauth.ManagedGitCredentialHelper
	request["GIT_CONFIG_KEY_3"] = "credential.useHttpPath"
	request["GIT_CONFIG_VALUE_3"] = "true"
	require.NoError(t, mgr.Configure("echo", nil, false, request, "", nil, false))

	for _, repository := range []string{"allowed", "foreign"} {
		cmd := exec.Command(git, "credential", "fill")
		cmd.Dir = root
		cmd.Env = append([]string(nil), cfg.AgentEnv...)
		cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=owner/" + repository + ".git\n\n")
		output, err := cmd.Output()
		if repository == "allowed" {
			require.NoError(t, err)
			require.Contains(t, string(output), "username=synthetic")
		} else {
			require.Error(t, err)
			require.Empty(t, output)
		}
	}
	require.NoFileExists(t, ambientMarker, "an ambient helper before the managed reset must not run")

	cmd := exec.Command(bash, "-c", "command -v gh")
	cmd.Env = append([]string(nil), cfg.AgentEnv...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.Equal(t, ghShim+"\n", string(output))
	marker, err := os.ReadFile(parentMarker)
	require.NoError(t, err)
	require.Equal(t, "ran", string(marker))
	require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "old-lease")
	require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "old-repository")
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestManagerConfigureManagedGitToolsMalformedEnvironmentIsAtomic(t *testing.T) {
	shimDir := filepath.Join(t.TempDir(), "installed shims")
	startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
	parentEnv := filepath.Join(t.TempDir(), "parent.sh")
	cfg := &config.InstanceConfig{
		WorkDir:      t.TempDir(),
		AgentCommand: "old-command",
		AgentEnv: []string{
			"PATH=" + shimDir + string(filepath.ListSeparator) + "/usr/bin",
			"BASH_ENV=" + startupEnv,
			githubauth.CredentialBrokerURLEnv + "=https://old-broker.example/resolve",
			githubauth.CredentialLeaseEnv + "=old-lease",
			githubauth.CredentialHelperPathEnv + "=/installed/agentctl",
			githubauth.CredentialCLIShimDirEnv + "=" + shimDir,
			githubauth.CredentialCLIBashEnvEnv + "=" + startupEnv,
			githubauth.CredentialParentBashEnv + "=" + parentEnv,
		},
	}
	mgr := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	before := append([]string(nil), cfg.AgentEnv...)
	err := mgr.Configure("new-command", nil, false, map[string]string{
		githubauth.CredentialBrokerURLEnv: "https://current-broker.example/resolve",
		githubauth.CredentialLeaseEnv:     "current-lease",
		"GIT_CONFIG_COUNT":                "2",
		"GIT_CONFIG_KEY_0":                "core.hooksPath",
	}, "", nil, false)
	require.Error(t, err)
	require.Equal(t, "old-command", cfg.AgentCommand)
	require.Equal(t, before, cfg.AgentEnv)
}

func managedGitToolContract(lease string) map[string]string {
	return map[string]string{
		githubauth.CredentialBrokerURLEnv:  "https://broker.example/resolve",
		githubauth.CredentialLeaseEnv:      lease,
		githubauth.CredentialRepositoryEnv: "current-repository",
		githubauth.CredentialOwnerEnv:      "current-owner",
		githubauth.CredentialHelperPathEnv: "",
		"GIT_CONFIG_COUNT":                 "3",
		"GIT_CONFIG_KEY_0":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":               "",
		"GIT_CONFIG_KEY_1":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":               githubauth.ManagedGitCredentialHelper,
		"GIT_CONFIG_KEY_2":                 "credential.useHttpPath",
		"GIT_CONFIG_VALUE_2":               "true",
	}
}
