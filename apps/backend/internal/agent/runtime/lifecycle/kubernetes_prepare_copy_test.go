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

func TestKubernetesPrepareScriptUpgradesPersistedManagedScripts(t *testing.T) {
	full, err := os.ReadFile("../../../../../../k8s/worker-images/full/prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	legacyDefault := persistedKubernetesPrepareScriptBeforePVCFix(t, DefaultPrepareScript("k8s"))
	for name, current := range map[string]string{
		"default": DefaultPrepareScript("k8s"),
		"full":    string(full),
	} {
		t.Run(name, func(t *testing.T) {
			legacy := persistedKubernetesPrepareScriptBeforePVCFix(t, current)
			if upgraded := upgradeLegacyKubernetesPrepareScript(legacy); upgraded != current {
				t.Fatalf("managed script upgrade did not produce the current script")
			}
			req := validKubernetesCreateRequest()
			req.Metadata[MetadataKeySetupScript] = legacy

			resolved, err := kubernetesPrepareScript(req)

			requireNoError(t, err)
			if !strings.Contains(resolved, "normalize_repository_origin()") ||
				!strings.Contains(resolved, `cp -R "$clone_tmp"/. "$workspace"/`) {
				t.Fatalf("persisted managed script was not upgraded:\n%s", resolved)
			}
			if strings.Contains(resolved, `cp -a "$clone_tmp"/. "$workspace"/`) {
				t.Fatalf("persisted managed script retained archive copy:\n%s", resolved)
			}
		})
	}

	const customization = "# user-managed customization"
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeySetupScript] = legacyDefault + "\n" + customization
	resolved, err := kubernetesPrepareScript(req)
	requireNoError(t, err)
	if !strings.Contains(resolved, customization) ||
		!strings.Contains(resolved, `cp -a "$clone_tmp"/. "$workspace"/`) ||
		strings.Contains(resolved, "normalize_repository_origin()") {
		t.Fatalf("custom prepare script was modified:\n%s", resolved)
	}
}

func persistedKubernetesPrepareScriptBeforePVCFix(t *testing.T, current string) string {
	t.Helper()
	legacy := strings.Replace(current, `
normalize_repository_origin() {
  printf '%s\n' "$1" | sed \
    -e 's|^https://[^/@]*@github.com/|https://github.com/|' \
    -e 's|^git@github.com:|https://github.com/|' \
    -e 's|^ssh://git@github.com/|https://github.com/|'
}
`, "", 1)
	legacy = strings.Replace(legacy, `    expected_origin=$(normalize_repository_origin "$repository_url")
    retained_origin=$(normalize_repository_origin "$workspace_origin")`, `    expected_origin=$(printf '%s\n' "$repository_url" | sed 's|^https://[^/@]*@github.com/|https://github.com/|')
    retained_origin=$(printf '%s\n' "$workspace_origin" | sed 's|^https://[^/@]*@github.com/|https://github.com/|')`, 1)
	legacy = strings.Replace(legacy,
		`cp -R "$clone_tmp"/. "$workspace"/`,
		`cp -a "$clone_tmp"/. "$workspace"/`, 1)
	if legacy == current {
		t.Fatal("legacy fixture did not reverse the managed PVC preparation changes")
	}
	return legacy
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
