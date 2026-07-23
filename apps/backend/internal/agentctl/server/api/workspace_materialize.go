package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/securityutil"
	"go.uber.org/zap"
)

// MaterializeRepositoryRequest describes one repository checkout below the
// agentctl workspace. RepositoryURL must be a credential-free Git locator;
// destination is always a direct child of the current workspace root.
type MaterializeRepositoryRequest struct {
	RepositoryURL  string `json:"repository_url"`
	Destination    string `json:"destination"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch,omitempty"`
}

// MaterializeRepositoryResponse deliberately contains no remote locator so a
// credential accidentally supplied by an untrusted caller cannot be echoed.
type MaterializeRepositoryResponse struct {
	Destination string `json:"destination"`
	Reused      bool   `json:"reused,omitempty"`
	Error       string `json:"error,omitempty"`
}

// RemoveMaterializedRepositoryRequest identifies a previously materialized,
// credential-free checkout that may be removed during a failed batch rollback.
type RemoveMaterializedRepositoryRequest struct {
	RepositoryURL string `json:"repository_url"`
	Destination   string `json:"destination"`
}

type removeMaterializedRepositoryResponse struct {
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleWorkspaceMaterializeRepository(c *gin.Context) {
	var req MaterializeRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MaterializeRepositoryResponse{Error: "invalid request"})
		return
	}
	destination, err := materializeDestination(s.procMgr.WorkDir(), req.Destination)
	if err != nil {
		c.JSON(http.StatusBadRequest, MaterializeRepositoryResponse{Error: "invalid destination"})
		return
	}
	if err := validateRepositoryLocator(req.RepositoryURL); err != nil {
		c.JSON(http.StatusBadRequest, MaterializeRepositoryResponse{Error: "invalid repository locator"})
		return
	}
	if !securityutil.IsValidBranchName(req.BaseBranch) || (req.CheckoutBranch != "" && !securityutil.IsValidBranchName(req.CheckoutBranch)) {
		c.JSON(http.StatusBadRequest, MaterializeRepositoryResponse{Error: "invalid repository branch"})
		return
	}

	reused, err := materializeRepository(c.Request.Context(), req.RepositoryURL, destination, req.BaseBranch, req.CheckoutBranch)
	if err != nil {
		if errors.Is(err, errMaterializeCollision) {
			c.JSON(http.StatusConflict, MaterializeRepositoryResponse{Error: "destination already exists"})
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			c.JSON(http.StatusRequestTimeout, MaterializeRepositoryResponse{Error: "repository materialization cancelled"})
			return
		}
		s.logger.Warn("workspace repository materialization failed", zap.String("destination", req.Destination), zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, MaterializeRepositoryResponse{Error: "repository materialization failed"})
		return
	}
	status := http.StatusCreated
	if reused {
		status = http.StatusOK
	}
	c.JSON(status, MaterializeRepositoryResponse{Destination: req.Destination, Reused: reused})
}

func (s *Server) handleWorkspaceRemoveMaterializedRepository(c *gin.Context) {
	var req RemoveMaterializedRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, removeMaterializedRepositoryResponse{Error: "invalid request"})
		return
	}
	destination, err := materializeDestination(s.procMgr.WorkDir(), req.Destination)
	if err != nil {
		c.JSON(http.StatusBadRequest, removeMaterializedRepositoryResponse{Error: "invalid destination"})
		return
	}
	if err := validateRemovalRepositoryLocator(req.RepositoryURL); err != nil {
		c.JSON(http.StatusBadRequest, removeMaterializedRepositoryResponse{Error: "invalid repository locator"})
		return
	}
	removed, err := removeMaterializedRepository(c.Request.Context(), s.procMgr.WorkDir(), destination, req.RepositoryURL)
	if err != nil {
		if errors.Is(err, errMaterializeCollision) {
			c.JSON(http.StatusConflict, removeMaterializedRepositoryResponse{Error: "destination is not the requested checkout"})
			return
		}
		s.logger.Warn("workspace repository cleanup failed", zap.String("destination", req.Destination), zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, removeMaterializedRepositoryResponse{Error: "repository cleanup failed"})
		return
	}
	c.JSON(http.StatusOK, removeMaterializedRepositoryResponse{Removed: removed})
}

var errMaterializeCollision = errors.New("materialize destination collision")

var beforeMaterializeQuarantineRename = func() {}

var afterMaterializeQuarantineOpen = func(string) {}

func materializeDestination(workDir, destination string) (string, error) {
	if destination == "" || filepath.IsAbs(destination) || filepath.Base(destination) != destination || destination == "." || destination == ".." {
		return "", errors.New("unsafe destination")
	}
	root, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	destination = filepath.Join(root, destination)
	if filepath.Dir(destination) != root {
		return "", errors.New("destination escapes workspace")
	}
	return destination, nil
}

func validateRepositoryLocator(locator string) error {
	if malformedRepositoryLocator(locator) {
		return errors.New("empty or malformed locator")
	}
	if filepath.IsAbs(locator) {
		return errors.New("local locator is not allowed")
	}
	parsed, err := url.Parse(locator)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("credentialed or malformed locator")
	}
	if isNetworkRepositoryScheme(parsed.Scheme) {
		return validateNetworkRepositoryLocator(parsed)
	}
	// Git's SCP-like syntax permits only the conventional git user; accepting
	// arbitrary users would make it impossible to distinguish credentials.
	if strings.HasPrefix(locator, "git@") && strings.Count(locator, ":") == 1 && !strings.ContainsAny(locator, " \\t") {
		return nil
	}
	return errors.New("unsupported locator")
}

func malformedRepositoryLocator(locator string) bool {
	return locator == "" || strings.TrimSpace(locator) != locator || strings.IndexFunc(locator, unicode.IsControl) >= 0
}

func isNetworkRepositoryScheme(scheme string) bool {
	switch scheme {
	case "https", "http", "ssh", "git":
		return true
	default:
		return false
	}
}

func validateNetworkRepositoryLocator(locator *url.URL) error {
	if locator.Host == "" {
		return errors.New("locator host required")
	}
	return nil
}

// validateRemovalRepositoryLocator keeps rollback capable of recognizing
// legacy local-test checkouts. Removal never opens the locator: it only
// compares it with an existing checkout's origin before deleting that owned
// destination, unlike the materialize endpoint which must not fetch local or
// file URLs from an HTTP caller.
func validateRemovalRepositoryLocator(locator string) error {
	if filepath.IsAbs(locator) || strings.HasPrefix(locator, "file:") {
		return nil
	}
	return validateRepositoryLocator(locator)
}

func materializeRepository(ctx context.Context, locator, destination, baseBranch, checkoutBranch string) (bool, error) {
	if reused, err := matchingCheckout(ctx, destination, locator, baseBranch, checkoutBranch); err != nil || reused {
		return reused, err
	}
	// codeql[go/path-injection] destination is a direct child of the canonical workspace root; Lstat rejects links before use.
	if _, err := os.Lstat(destination); err == nil {
		return false, errMaterializeCollision
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect destination: %w", err)
	}
	parent := filepath.Dir(destination)
	// codeql[go/path-injection] parent is the canonical workspace root containing the direct-child destination.
	tmp, err := os.MkdirTemp(parent, ".kandev-clone-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	checkout := filepath.Join(tmp, "checkout")
	if _, err := materializeGitOutput(ctx, "clone", "--no-checkout", "--", locator, checkout); err != nil {
		return false, err
	}
	if err := checkoutMaterializedBranch(ctx, checkout, baseBranch, checkoutBranch); err != nil {
		return false, err
	}
	// codeql[go/path-injection] checkout is newly created beneath the trusted workspace root; destination is its direct child.
	if err := os.Rename(checkout, destination); err != nil {
		if os.IsExist(err) {
			return false, errMaterializeCollision
		}
		return false, err
	}
	return false, nil
}

func checkoutMaterializedBranch(ctx context.Context, checkout, baseBranch, checkoutBranch string) error {
	branch := checkoutBranch
	if branch == "" {
		branch = baseBranch
	}
	if hasGitRef(ctx, checkout, "refs/heads/"+branch) {
		_, err := materializeGitOutput(ctx, "-C", checkout, "checkout", branch)
		return err
	}
	if hasGitRef(ctx, checkout, "refs/remotes/origin/"+branch) {
		_, err := materializeGitOutput(ctx, "-C", checkout, "checkout", "--track", "-b", branch, "origin/"+branch)
		return err
	}
	if checkoutBranch == "" {
		return errors.New("base branch is unavailable from origin")
	}
	_, err := materializeGitOutput(ctx, "-C", checkout, "checkout", "-b", checkoutBranch, "origin/"+baseBranch)
	return err
}

func hasGitRef(ctx context.Context, directory, ref string) bool {
	_, err := materializeGitOutput(ctx, "-C", directory, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func matchingCheckout(ctx context.Context, destination, locator, baseBranch, checkoutBranch string) (bool, error) {
	// codeql[go/path-injection] destination is a direct child of the canonical workspace root and must be a real directory before Git probes.
	destinationInfo, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir() {
		return false, errMaterializeCollision
	}
	gitPath := filepath.Join(destination, ".git")
	// codeql[go/path-injection] .git is the exact child of a real, non-symlink destination and is Lstat-checked before Git probes.
	gitInfo, err := os.Lstat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errMaterializeCollision
	}
	if gitInfo.Mode()&os.ModeSymlink != 0 || !gitInfo.IsDir() {
		return false, errMaterializeCollision
	}
	origin, err := materializeGitOutput(ctx, "-C", destination, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(origin) != locator {
		return false, errMaterializeCollision
	}
	branch := checkoutBranch
	if branch == "" {
		branch = baseBranch
	}
	currentBranch, err := materializeGitOutput(ctx, "-C", destination, "branch", "--show-current")
	if err != nil || strings.TrimSpace(currentBranch) != branch {
		return false, errMaterializeCollision
	}
	requestedRef := "origin/" + branch
	if !hasGitRef(ctx, destination, "refs/remotes/"+requestedRef) {
		requestedRef = "origin/" + baseBranch
	}
	requestedCommit, err := materializeGitOutput(ctx, "-C", destination, "rev-parse", "--verify", requestedRef+"^{commit}")
	if err != nil {
		return false, errMaterializeCollision
	}
	headCommit, err := materializeGitOutput(ctx, "-C", destination, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(headCommit) != strings.TrimSpace(requestedCommit) {
		return false, errMaterializeCollision
	}
	return true, nil
}

func removeMaterializedRepository(ctx context.Context, workDir, destination, locator string) (bool, error) {
	canonicalWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return false, err
	}
	if filepath.Dir(destination) != canonicalWorkDir {
		return false, errMaterializeCollision
	}
	root, err := os.OpenRoot(canonicalWorkDir)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	quarantine, err := materializeQuarantine(root)
	if err != nil {
		return false, err
	}
	destinationName := filepath.Base(destination)
	beforeMaterializeQuarantineRename()
	// codeql[go/path-injection] Root binds both direct-child names to the canonical workspace; quarantine captures the exact object before validation.
	if err := root.Rename(destinationName, quarantine); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	// codeql[go/path-injection] Root Lstat rejects a link or non-directory before opening the quarantined name.
	quarantineInfo, err := root.Lstat(quarantine)
	if err != nil || quarantineInfo.Mode()&os.ModeSymlink != 0 || !quarantineInfo.IsDir() {
		return false, restoreMaterializedQuarantine(root, quarantineInfo, quarantine, destinationName)
	}
	// codeql[go/path-injection] Root opens the exact object atomically moved into its private canonical-workdir quarantine.
	quarantineRoot, err := root.OpenRoot(quarantine)
	if err != nil {
		return false, restoreMaterializedQuarantine(root, nil, quarantine, destinationName)
	}
	defer func() { _ = quarantineRoot.Close() }()
	capturedInfo, err := quarantineRoot.Lstat(".")
	if err != nil || capturedInfo.Mode()&os.ModeSymlink != 0 || !capturedInfo.IsDir() {
		return false, restoreMaterializedQuarantine(root, capturedInfo, quarantine, destinationName)
	}
	afterMaterializeQuarantineOpen(quarantine)
	// codeql[go/path-injection] The opened quarantine root keeps .git bound to the captured directory, independent of later pathname replacement.
	gitDir, err := quarantineRoot.Lstat(".git")
	if err != nil || gitDir.Mode()&os.ModeSymlink != 0 || !gitDir.IsDir() {
		return false, restoreMaterializedQuarantine(root, capturedInfo, quarantine, destinationName)
	}
	origin, err := materializeQuarantineOrigin(quarantineRoot)
	if err != nil || strings.TrimSpace(origin) != locator {
		return false, restoreMaterializedQuarantine(root, capturedInfo, quarantine, destinationName)
	}
	// codeql[go/path-injection] The captured quarantine root recursively removes only its own entries after origin verification.
	if err := clearMaterializedQuarantine(quarantineRoot); err != nil {
		return false, err
	}
	if matches, err := quarantineMatchesCaptured(root, quarantine, capturedInfo); err != nil || !matches {
		return false, errMaterializeCollision
	}
	// codeql[go/path-injection] Root non-recursively removes the quarantined parent entry only after it still matches the captured object.
	if err := root.Remove(quarantine); err != nil {
		return false, err
	}
	return true, nil
}

func materializeQuarantine(root *os.Root) (string, error) {
	for range 10 {
		entropy := make([]byte, 16)
		if _, err := rand.Read(entropy); err != nil {
			return "", err
		}
		name := fmt.Sprintf(".kandev-remove-%x", entropy)
		if err := root.Mkdir(name, 0o700); err == nil {
			if err := root.Remove(name); err != nil {
				return "", err
			}
			return name, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("allocate removal quarantine")
}

func restoreMaterializedQuarantine(root *os.Root, captured os.FileInfo, quarantine, destination string) error {
	if captured == nil {
		return errMaterializeCollision
	}
	if matches, err := quarantineMatchesCaptured(root, quarantine, captured); err != nil || !matches {
		return errMaterializeCollision
	}
	if _, err := root.Lstat(destination); err == nil {
		return errMaterializeCollision
	} else if !os.IsNotExist(err) {
		return err
	}
	// codeql[go/path-injection] Root restores the exact quarantined object only when its direct-child destination remains absent.
	if err := root.Rename(quarantine, destination); err != nil {
		return err
	}
	return errMaterializeCollision
}

func quarantineMatchesCaptured(root *os.Root, quarantine string, captured os.FileInfo) (bool, error) {
	info, err := root.Lstat(quarantine)
	if err != nil {
		return false, err
	}
	return os.SameFile(info, captured), nil
}

func materializeQuarantineOrigin(root *os.Root) (string, error) {
	config, err := root.ReadFile(filepath.Join(".git", "config"))
	if err != nil {
		return "", err
	}
	return parseMaterializeOriginURL(string(config)), nil
}

func parseMaterializeOriginURL(config string) string {
	inOrigin := false
	for line := range strings.SplitSeq(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			fields := strings.Fields(strings.TrimSpace(line[1 : len(line)-1]))
			inOrigin = len(fields) == 2 && strings.EqualFold(fields[0], "remote") && fields[1] == `"origin"`
			continue
		}
		if !inOrigin || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "url") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clearMaterializedQuarantine(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := root.RemoveAll(entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func materializeGitOutput(ctx context.Context, args ...string) (string, error) {
	// codeql[go/command-injection] git is fixed; HTTP locators and refs are validated before checkout and matching probes.
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- git is fixed and arguments are validated; no shell.
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("git command failed")
	}
	return string(output), nil
}
