package backendapp

import (
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/config"
	"path/filepath"
)

func newSandboxPolicyService(cfg *config.Config, database *sqlx.DB) (*sandbox.Service, error) {
	roots := []string{cfg.ResolvedDataDir(), filepath.Join(cfg.ResolvedHomeDir(), "plugins"), filepath.Join(cfg.ResolvedHomeDir(), "config.yaml")}
	for _, path := range []string{cfg.Source.FilePath, cfg.Database.Path} {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		roots = append(roots, absolute)
		if path == cfg.Database.Path {
			roots = append(roots, absolute+"-wal", absolute+"-shm")
		}
	}
	return sandbox.NewService(database, cfg.Features.SandboxPolicies, roots)
}
