package backendapp

import (
	"errors"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// installProviderErrorCatalog loads the configured operator catalog once. A
// rejected catalog is logged and ignored: startup continues with the
// built-in rules only.
func installProviderErrorCatalog(cfg *config.Config, log *logger.Logger) {
	path := providerErrorCatalogPath(cfg)
	catalog, err := externalcatalog.Load(path, routingerr.ExternalCatalogVocabulary())
	if err != nil {
		routingerr.SetExternalCatalog(nil)
		fields := []zap.Field{zap.String("reason", "unknown")}
		var loadErr *externalcatalog.LoadError
		if errors.As(err, &loadErr) {
			fields = []zap.Field{zap.String("reason", loadErr.Reason), zap.Int("rule_index", loadErr.Index),
				zap.String("rule_id", loadErr.RuleID), zap.String("field", loadErr.Field)}
		}
		log.Warn("provider error catalog rejected; using built-in rules only", fields...)
		return
	}
	routingerr.SetExternalCatalog(catalog)
	if catalog != nil {
		log.Info("provider error catalog loaded", zap.Int("rules", catalog.Len()))
	}
}

// providerErrorCatalogPath resolves a relative path against the directory of
// the selected config file, or the working directory when none was selected.
func providerErrorCatalogPath(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	path := strings.TrimSpace(cfg.ProviderErrors.CatalogFile)
	if path == "" || filepath.IsAbs(path) || cfg.Source.FilePath == "" {
		return path
	}
	return filepath.Join(filepath.Dir(cfg.Source.FilePath), path)
}
