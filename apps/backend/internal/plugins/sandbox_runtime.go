package plugins

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type SandboxRuntimeBinary struct {
	Content       []byte `json:"-"`
	Digest        string `json:"digest"`
	PackageDigest string `json:"package_digest"`
}

// ReadSandboxRuntime reads a verified executable from the selected installation.
func (s *Service) ReadSandboxRuntime(ctx context.Context, binding SandboxProviderBinding, artifact *pluginsdk.SandboxRuntimeArtifact) (*SandboxRuntimeBinary, error) {
	if !s.sandboxEnabled() || !validSandboxRuntimeArtifact(artifact) || len(binding.PackageDigest) != 64 {
		return nil, ErrSandboxProviderUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	record, release, err := s.admitSandboxProvider(binding)
	if err != nil {
		return nil, err
	}
	defer release()
	ctx, finish, err := s.beginProviderDispatch(ctx, binding.PluginID)
	if err != nil {
		return nil, ErrSandboxProviderUnavailable
	}
	defer finish()
	content, err := pkgtar.ReadInstalledFile(ctx, record.InstallPath, pkgtar.InstalledFileSpec{
		Name: artifact.RelativePath, PackageDigest: record.PackageDigest, Digest: artifact.Sha256, MaxBytes: 128 << 20,
	})
	if err != nil || len(content) == 0 || ctx.Err() != nil {
		return nil, errors.Join(ErrSandboxProviderUnavailable, ctx.Err())
	}
	return &SandboxRuntimeBinary{Content: content, Digest: artifact.Sha256, PackageDigest: record.PackageDigest}, nil
}
