package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func installMetadataRestrictedCopy(t *testing.T, bin string) {
	t.Helper()
	realCopy, err := exec.LookPath("cp")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    -a|-p|--archive|--preserve|--preserve=*)
      echo 'cp: preserving metadata: Operation not permitted' >&2
      exit 1
      ;;
  esac
done
exec ` + shellQuote(realCopy) + ` "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "cp"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestKubernetesPreparationAcceptsEquivalentGitHubOrigins(t *testing.T) {
	full, err := os.ReadFile("../../../../../../k8s/worker-images/full/prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"default": DefaultPrepareScript("k8s"),
		"full":    string(full),
	} {
		t.Run(name, func(t *testing.T) {
			runKubernetesPreparationWithEquivalentGitHubOrigins(t, script, name == "full")
		})
	}
}

func runKubernetesPreparationWithEquivalentGitHubOrigins(t *testing.T, script string, needsDocker bool) {
	t.Helper()
	workspace, _ := setupPostludeRepo(t, "main")
	runIn(t, workspace, "git", "remote", "set-url", "origin", "https://github.com/acme/repo.git")
	marker := filepath.Join(workspace, "retained.txt")
	requireNoError(t, os.WriteFile(marker, []byte("preserve me"), 0o600))
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	requireNoError(t, os.MkdirAll(home, 0o700))
	requireNoError(t, os.MkdirAll(bin, 0o755))
	if needsDocker {
		requireNoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}

	rendered := strings.NewReplacer(
		"{{git.identity_setup}}", ":",
		"{{github.auth_setup}}", ":",
		"{{repository.branch}}", shellQuote("main"),
		"{{repository.clone_url}}", shellQuote("git@github.com:acme/repo.git"),
		"{{workspace.path}}", shellQuote(workspace),
		"{{repository.setup_script}}", ":",
		"{{kandev.agents.install}}", ":",
		"/opt/kandev/.workspace-clone", filepath.Join(root, "runtime", "workspace-clone"),
	).Replace(script)
	cmd := exec.Command("sh", "-eu", "-c", rendered)
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Kubernetes prepare rejected equivalent GitHub origins: %v\n%s", err, output)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "preserve me" {
		t.Fatalf("retained workspace data = %q, %v", data, err)
	}
}
