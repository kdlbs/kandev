package backendapp

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/secrets"
	"go.uber.org/zap"
)

const (
	gitCredentialReissueSignerInstallation = "installation"
	gitCredentialReissueSignerConfigured   = "configured"
	gitCredentialReissueSignerDisabled     = "disabled"
	gitCredentialReissueKeyInfo            = "kandev.gitcredentials.reissue.installation.v1"
)

func logGitCredentialReissueSignerSelected(log *logger.Logger, source string) {
	if log != nil {
		log.Info("git_credential_reissue.signer_selected", zap.String("source", source))
	}
}

func selectGitCredentialReissueSigner(
	cfg config.GitHubCredentialBrokerConfig,
	masterKey []byte,
) (*gitcredentials.ReissueCapabilitySigner, string, error) {
	if cfg.DisableReissue {
		return nil, gitCredentialReissueSignerDisabled, nil
	}
	if strings.TrimSpace(cfg.ReissueSigningKey) != "" {
		signer, err := gitcredentials.NewReissueCapabilitySigner(cfg.ReissueSigningKey)
		if err != nil {
			return nil, "", fmt.Errorf("initialize configured Git credential reissue signer: %w", err)
		}
		return signer, gitCredentialReissueSignerConfigured, nil
	}
	if len(masterKey) != secrets.MasterKeySize {
		return nil, "", fmt.Errorf("installation master key has invalid length")
	}
	derivedKey, err := hkdf.Key(sha256.New, masterKey, nil, gitCredentialReissueKeyInfo, 32)
	if err != nil {
		return nil, "", fmt.Errorf("derive installation Git credential reissue key: %w", err)
	}
	signingKey := base64.RawURLEncoding.EncodeToString(derivedKey)
	clear(derivedKey)
	signer, err := gitcredentials.NewReissueCapabilitySigner(signingKey)
	if err != nil {
		return nil, "", fmt.Errorf("initialize installation Git credential reissue signer: %w", err)
	}
	return signer, gitCredentialReissueSignerInstallation, nil
}
