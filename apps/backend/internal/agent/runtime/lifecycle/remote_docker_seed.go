package lifecycle

import (
	"context"
	"fmt"
	"runtime"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
)

// seedRemoteAgentSessionDir copies the agent's login files and selected
// configuration bundles into the per-instance directory on the remote host.
//
// This is the local Docker seeder with two substitutions: the writer is SFTP
// rather than the local filesystem, and the destination is the remote
// per-instance directory rather than one under the backend's Kandev home. Both
// paths call the same UploadCredentialFiles and UploadPortableConfigBundles.
//
// The destination matters. The SSH executor writes into the remote user's home
// because its agent runs there directly. A remote Docker agent runs inside a
// container and sees only the mounted per-instance directory, so files in the
// remote home would never reach it.
func seedRemoteAgentSessionDir(
	ctx context.Context,
	client *ssh.Client,
	ag agents.Agent,
	remoteSessionDir string,
	selectedBundleIDs []string,
	log *logger.Logger,
	onWarnings func([]PortableConfigWarning),
) error {
	if ag == nil {
		return nil
	}
	if remoteSessionDir == "" {
		return fmt.Errorf("remote docker: session dir is required to seed agent credentials")
	}
	if client == nil {
		return fmt.Errorf("remote docker: no SSH connection to seed agent credentials")
	}

	uploader := &sshFileUploader{client: client}

	var authErr error
	if auth := ag.RemoteAuth(); auth != nil {
		// Source files are read from the backend host, so the host OS
		// selects which of the agent's declared source lists applies.
		methods := authMethodsForHost(auth.Methods, runtime.GOOS)
		if len(methods) > 0 {
			authErr = UploadCredentialFiles(ctx, uploader, methods, remoteSessionDir, log)
			if authErr != nil && log != nil {
				log.Warn("remote docker: credential files failed; continuing with configuration bundles",
					zap.Error(authErr))
			}
		}
	}

	if len(selectedBundleIDs) > 0 {
		warnings := UploadPortableConfigBundles(ctx, uploader, ag, selectedBundleIDs, remoteSessionDir, log)
		if onWarnings != nil {
			onWarnings(warnings)
		}
	}

	return authErr
}
