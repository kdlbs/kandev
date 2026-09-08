---
id: "01-configuration-catalog-and-source-resolution"
title: "Configuration catalog and source resolution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 01: Configuration catalog and source resolution

Build the shared typed contract before changing consumers. Add stable YAML
fields, source metadata, file discovery, validation, and secret permission
diagnostics to the common configuration package.

## Acceptance

- A canonical catalog records YAML key, compatible environment variables,
  sensitivity, owner, and default policy for every stable startup setting.
- A reviewed exclusion list covers internal, generated, test, debug-only,
  deprecated, platform-detection, and externally managed variables.
- Completeness tests fail when a stable environment setting has neither a
  catalog entry nor an exclusion.
- Typed configuration includes every new key listed in the spec.
- Source resolution checks working directory, resolved home, then
  `/etc/kandev`. It loads one file and never merges candidates.
- The first existing unreadable or invalid file causes a clear startup error.
- A selected home configuration file rejects `homeDir`. Working-directory and
  system files can set it.
- Selected-file metadata records the path and whether a YAML key supplied each
  resolved value.
- Secret-bearing files with broad Unix read permissions produce a warning that
  contains the path but no value. Secure files and files without secrets do not
  warn.
- Existing environment bindings and profile defaults keep their precedence and
  behavior.

## Files likely touched

- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`
- `apps/backend/internal/common/config/catalog.go` (new)
- `apps/backend/internal/common/config/catalog_test.go` (new)
- `apps/backend/internal/common/config/source.go` (new)
- `apps/backend/internal/common/config/source_test.go` (new)

## Dependencies

None.

## TDD sequence

1. Add table tests for catalog completeness, aliases, sensitivity, source
   precedence, home bootstrap, invalid first candidates, relocation rejection,
   source provenance, and Unix permission warnings. Run them RED.
2. Add typed sections and catalog metadata. Implement file selection without
   changing the current environment and profile precedence.
3. Add secret-aware permission diagnostics that never format secret values.
4. Run the focused tests GREEN. Run the complete common configuration and
   runtime flag suites to prove registry compatibility.

## Verification

```bash
cd apps/backend && go test ./internal/common/config -run '^Test(ConfigurationCatalog|ConfigSource|ConfigPrecedence|HomeConfig|SecretConfigPermissions)' -count=1
cd apps/backend && go test ./internal/common/config ./internal/runtimeflags -count=1
```

## Risks

- A broad environment scan can accidentally define internal variables as public
  contract. The exclusion reason must be explicit and testable.
- Logging a decoded configuration object can expose secrets. Diagnostics must
  use catalog metadata and paths only.
- Viper aliases and mapstructure names can drift. Catalog completeness tests
  must compare the actual typed bindings.

## Output contract

Record RED and GREEN results, the final catalog shape, every exclusion class,
permission warning evidence, files changed, and remaining risks in `## Results`.

## Results

RED:

- `go test ./internal/common/config -run 'TestLoadReadsStableYAMLFields|TestLoadDiscoversHomeConfiguration|TestHomeConfigurationCannotRelocateItself|TestConfigSourceMetadataReportsSelectedFileAndYAMLValues' -count=1`
  failed on the missing typed fields, home discovery, relocation guard, and
  source metadata as expected.
- `TestInvalidYAMLRangeNamesSettingAndFile` also failed before range validation
  was added.

GREEN:

- `go test ./internal/common/config -run '^Test(ConfigurationCatalog|ConfigSource|ConfigPrecedence|HomeConfig|SecretConfigPermissions)' -count=1`
  passed with 11 tests.
- `go test ./internal/common/config ./internal/runtimeflags -count=1`
  passed with 108 tests.

The shared contract now exposes typed task, credential, capacity, queue,
agentctl, planning, observability, and launcher sections. `ConfigurationCatalog`
records canonical keys, environment aliases, owners, defaults, and sensitivity.
`ConfigurationExclusions` records internal wiring, generated values, packaging,
workspace injection, profile, test, and debug-only variables. `ConfigSource`
records the selected file, per-key source, and path-only permission warnings.

Discovery uses working directory, resolved home, then `/etc/kandev`; it selects
one existing file and does not merge or fall through after read or validation
errors. The selected home file rejects `homeDir`. YAML values are decoded
without copying them into the process environment, while explicit environment
aliases retain override priority and legacy invalid-value fallback behavior.
Secret-bearing files with broad Unix read permissions emit a path-only warning
recommending mode `0600`.

Files changed:

- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/catalog.go`
- `apps/backend/internal/common/config/catalog_test.go`
- `apps/backend/internal/common/config/source.go`
- `apps/backend/internal/common/config/source_test.go`
- `apps/backend/internal/common/config/validation.go`
- `apps/backend/internal/profiles/profiles.go`

Review remediation:

- The completeness test now compares catalog entries and exclusions with an
  independent audited startup-environment inventory.
- A synthetic uncataloged variable regression proves that the audit rejects
  omissions instead of relying on a minimum count or selected keys.
- GREEN: `go test ./internal/common/config -count=1` passed with 81 tests;
  `go test ./internal/common/config ./internal/runtimeflags -count=1` passed
  with 115 tests.
