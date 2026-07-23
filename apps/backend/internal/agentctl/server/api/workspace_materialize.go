package api

import (
	"context"
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
	if _, err := os.Lstat(destination); err == nil {
		return false, errMaterializeCollision
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect destination: %w", err)
	}
	parent := filepath.Dir(destination)
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
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
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
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, errMaterializeCollision
	}
	gitDir, err := os.Stat(filepath.Join(destination, ".git"))
	if err != nil || !gitDir.IsDir() {
		return false, errMaterializeCollision
	}
	origin, err := materializeGitOutput(ctx, "-C", destination, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(origin) != locator {
		return false, errMaterializeCollision
	}
	trash, err := os.MkdirTemp(workDir, ".kandev-remove-")
	if err != nil {
		return false, err
	}
	if err := os.Remove(trash); err != nil {
		return false, err
	}
	if err := os.Rename(destination, trash); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.RemoveAll(trash); err != nil {
		return false, err
	}
	return true, nil
}

func materializeGitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- validated locator/ref; no shell.
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("git command failed")
	}
	return string(output), nil
}
