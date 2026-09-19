package repoclone

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const checkoutCacheMarker = "kandev-checkout-cache.json"

type checkoutCacheIdentity struct {
	Version int    `json:"version"`
	Origin  string `json:"origin"`
	Mode    string `json:"mode"`
}

func hasCheckoutOptions(request GitCredentialRequest) bool {
	o := request.CheckoutOptions
	return o != nil && (o.Version != 1 || o.DownloadMode != models.DownloadStandard || len(o.SparseDirectories) > 0)
}

func (c *Cloner) ensureWorkspaceCheckoutCache(ctx context.Context, request GitCredentialRequest, credentialOrigin, token string) (string, RemoteRefState, error) {
	options, err := models.NormalizeRepositoryCheckoutOptions(request.CheckoutOptions)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	path, err := c.checkoutCachePath(request, options)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	cloneURL, auth, err := c.workspaceCloneAuthRequest(ctx, request, credentialOrigin, token)
	if err != nil {
		return "", RemoteRefStateUnknown, err
	}
	identity := checkoutCacheIdentity{Version: 1, Origin: cloneURL, Mode: options.DownloadMode}
	mu := c.repoMu(path)
	mu.Lock()
	defer mu.Unlock()
	if err := c.ensureCheckoutCache(ctx, path, identity, auth); err != nil {
		return path, RemoteRefStateUnknown, err
	}
	state, err := c.remoteRefState(ctx, cloneURL, auth)
	return path, state, err
}

func (c *Cloner) ensureCheckoutCache(ctx context.Context, path string, identity checkoutCacheIdentity, auth *cloneAuth) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("managed checkout cache path is not a directory")
		}
		data, err := os.ReadFile(filepath.Join(path, ".git", checkoutCacheMarker))
		var saved checkoutCacheIdentity
		if err != nil || json.Unmarshal(data, &saved) != nil || saved != identity {
			return errors.New("managed checkout cache is incomplete or incompatible")
		}
		if err := c.verifyOriginURLLocked(ctx, path, identity.Origin); err != nil {
			return err
		}
		c.fetch(ctx, path, auth)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".clone-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	checkout := filepath.Join(staging, "checkout")
	if err := c.cloneCheckoutCache(ctx, checkout, identity, auth); err != nil {
		return err
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git", checkoutCacheMarker), data, 0o600); err != nil {
		return err
	}
	return os.Rename(checkout, path)
}

func (c *Cloner) cloneCheckoutCache(ctx context.Context, path string, identity checkoutCacheIdentity, auth *cloneAuth) error {
	args := []string{"clone", "--no-checkout", gitNoTags}
	if identity.Mode == models.DownloadOnDemand {
		args = append(args, "--filter=blob:none")
	}
	args = append(args, "--", identity.Origin, path)
	out, err := c.runConfiguredGitCombined(ctx, gitCloneTimeout, args, func(cmd *exec.Cmd) (func(), error) { return configureGitCommand(cmd, auth) })
	if err != nil {
		return fmt.Errorf("repository cache clone failed: %s: %w", redactCloneOutput(string(out), authToken(auth)), err)
	}
	if identity.Mode == models.DownloadOnDemand && strings.Contains(strings.ToLower(string(out)), "filtering not recognized") {
		return errors.New("repository server does not support on-demand downloads")
	}
	return nil
}

func (c *Cloner) checkoutCachePath(request GitCredentialRequest, options *models.RepositoryCheckoutOptions) (string, error) {
	standardPath, err := c.WorkspaceProviderRepositoryPath(request.WorkspaceID, request.Provider, request.ProviderHost, request.ProviderScope, request.ProviderRepositoryID, request.Owner, request.Name)
	if err != nil {
		return "", err
	}
	base, err := c.ExpandedBasePath()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(standardPath))
	return filepath.Join(base, managedWorkspacesDir, request.WorkspaceID, "_checkout_modes", hex.EncodeToString(sum[:]), options.DownloadMode+"-v1"), nil
}
