package subproc

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

// gitPathMemo holds the Git executable resolved under one PATH value. A
// remembered path is reused only while PATH and PATHEXT are unchanged and the
// target remains executable; otherwise Git is resolved again and the record replaced.
var gitPathMemo struct {
	sync.Mutex
	path    string
	pathEnv string
	pathExt string
}

func rememberedGitPath(pathEnv, pathExt string) string {
	gitPathMemo.Lock()
	defer gitPathMemo.Unlock()
	if gitPathMemo.pathEnv != pathEnv || gitPathMemo.pathExt != pathExt {
		return ""
	}
	return gitPathMemo.path
}

func resolveGitPath(pathEnv, pathExt string) (string, error) {
	resolved, err := exec.LookPath("git")
	gitPathMemo.Lock()
	defer gitPathMemo.Unlock()
	if err != nil {
		gitPathMemo.path, gitPathMemo.pathEnv, gitPathMemo.pathExt = "", "", ""
		return "", err
	}
	gitPathMemo.path, gitPathMemo.pathEnv, gitPathMemo.pathExt = resolved, pathEnv, pathExt
	return resolved, nil
}

// GitExecutablePath resolves the Git binary for helper processes that need to
// preserve a file-descriptor-based working directory. Keeping the lookup in
// the Git seam lets the repository audit cover both command construction and
// executable discovery. A path remembered under the current PATH and PATHEXT is
// validated directly instead of repeating the directory search.
func GitExecutablePath() (string, error) {
	pathEnv, pathExt := os.Getenv("PATH"), os.Getenv("PATHEXT")
	if remembered := rememberedGitPath(pathEnv, pathExt); remembered != "" {
		if resolved, err := exec.LookPath(remembered); err == nil && resolved == remembered {
			return remembered, nil
		}
	}
	return resolveGitPath(pathEnv, pathExt)
}

// NewGitCommand is the only production Git command-construction seam. Callers
// must pass the returned command to a classified RunGit* helper (or hold a
// classified slot around a streaming Start/Wait lifecycle).
//
// Build the command with only the fixed executable at the exec.CommandContext
// sink, then attach the already-tokenized argv. The executable is the resolved
// Git path from GitExecutablePath, or "git" when Git cannot be resolved so exec
// reports its usual not-found error; argv[0] is always "git". Git never invokes
// a shell for Cmd.Args, and keeping the user-derived values out of the
// constructor also prevents CodeQL's command-injection query from treating this
// direct-argv seam as shell interpolation. Callers remain responsible for
// validating user-controlled refs, paths, and option values before they reach
// this seam.
func NewGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	name := "git"
	if resolved, err := GitExecutablePath(); err == nil {
		name = resolved
	}
	cmd := exec.CommandContext(ctx, name)
	cmd.Args = append([]string{"git"}, args...)
	return cmd
}
