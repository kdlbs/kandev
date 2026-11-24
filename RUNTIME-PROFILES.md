## Runtime profiles

`profiles.yaml` owns environment-driven runtime defaults. `profiles.ApplyProfile()` applies defaults only when the variable is not already set.

Select `e2e` with `KANDEV_E2E_MOCK=true`, or `dev` with `KANDEV_DEBUG_DEV_MODE=true`. Otherwise the profile is `prod`. `KANDEV_DEBUG_PPROF_ENABLED=true` does not select `dev`.

Runtime feature flags use this precedence: explicit environment variable, SQLite override, profile default. `apps/backend/internal/runtimeflags/registry.go` owns flag metadata and configuration. Do not add parallel registries. Use `/runtime-feature-flags` for any flag lifecycle change.

Stable operator settings use `apps/backend/internal/common/config/catalog.go` and `source.go`. Configuration discovery selects the first existing `config.yaml` in the working directory, home, then `/etc/kandev`. It does not merge files. Environment values override YAML. Pass typed values or explicit child contracts to consumers; do not copy YAML into public child environments.

For override and restart behavior, see `docs/decisions/0018-runtime-settings-overrides.md` and `docs/decisions/0019-restart-supervisor.md`.
