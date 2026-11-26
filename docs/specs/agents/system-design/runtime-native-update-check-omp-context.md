---
status: draft
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-002
created: 2026-11-19
owners:
  - Kandev
---

# OMP Native Update Check Context Design

## Purpose and mapping

This design declares the configuration inputs for Oh My Pi's native update
check, as used by the cache identity in the
[native agent update checks design](runtime-native-update-checks.md#status-integration-and-cache).
It satisfies the declared-configuration part of
AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2.

| Requirement | Design |
| --- | --- |
| REQ-AGENTS-RUNTIME-NATIVE-CHECK-002 | [Declared sources](#declared-sources), [Fingerprint](#fingerprint), [Bounded staleness](#bounded-staleness) |

## Declared sources

`ompUpdateCheckContext` declares the configuration sources that upstream
`omp update --check` reads at commit `2b562c5b7e3f` to select its channel,
installation ownership, and release feed. The sources are
`packages/utils/src/dirs.ts`, `packages/utils/src/env.ts`,
`packages/coding-agent/src/config/settings.ts`,
`packages/coding-agent/src/cli/managed-install.ts`,
`packages/coding-agent/src/cli/npm-registry.ts`, and
`packages/coding-agent/src/cli/build-service.ts`. Kandev resolves source paths
from the check's process environment only and does not parse dotenv files.
When the home variable is unset or empty, the home directory is the
effective account's home (`user.Current`), matching Node `os.homedir()`.
The working directory is the backend's physical directory (symlinks
evaluated), matching OMP's `process.cwd()`.
The dotenv files that upstream loads at startup are fingerprinted themselves,
so any edit to them changes the key. `TestOmpUpdateCheckContext` pins this
list to that commit.

- `EnvKeys`: `OMP_PROFILE`, `PI_PROFILE`, `PI_CODING_AGENT_DIR`,
  `PI_CONFIG_DIR`, `PI_CONFIG_FILES`, `HOME`, `XDG_CONFIG_HOME`,
  `npm_config_userconfig`, `npm_config_registry`, `BUN_CONFIG_REGISTRY`, the
  install-method selectors `MISE_DATA_DIR`, `XDG_DATA_HOME`, `LOCALAPPDATA`,
  and `BUN_INSTALL_GLOBAL_DIR` (upstream `getMiseDataDir` and
  `resolveUpdateTarget`, which choose between npm metadata and the build
  service), and `PI_BUILD_URL`, the build service origin (`buildServiceUrl`).
  Any change to these values changes the key.
- Channel settings: the profile is `OMP_PROFILE` when that variable is
  defined, even if it is empty, and otherwise `PI_PROFILE`. After trimming, an
  empty or `default` value selects the default profile. The base root is
  `<base>` = `$HOME/<PI_CONFIG_DIR or .omp>`. The active profile config root,
  equivalent to upstream `getConfigRootDir()`, is `<root>` =
  `<base>/profiles/<profile>` for a named profile and `<base>` for the default
  profile. A named profile uses `<root>/agent`. The default profile uses
  `$PI_CODING_AGENT_DIR` when it is set, unless that value equals the agent
  directory derived from the normalized `PI_PROFILE`. In that case it uses
  `<root>/agent`, matching upstream `resolvePreProfileAgentDir` and
  `isProfileDerivedAgentDir`. This applies both when `OMP_PROFILE` is
  explicitly empty or `default` and `PI_PROFILE` names another profile.
  Without an override, the default profile uses `<root>/agent`. An invalid
  profile name is fingerprinted as the default profile, matching upstream's
  module-load fallback. The check itself reports OMP's error.
  - Global files: `<agentDir>/config.yml` and `<agentDir>/config.yaml`, which
    are upstream `MAIN_CONFIG_FILENAMES`.
  - Overlay files: `PI_CONFIG_FILES` split on the OS path-list separator,
    with empty entries dropped and others kept verbatim, whitespace
    included. Each entry is expanded like upstream `expandTilde`: `~`, `~/`,
    `~\`, and a bare `~name` meaning `<home>/name`. Relative paths resolve
    against the working directory like Node `path.resolve`. On Windows a
    root-relative path (`\dir`) keeps the working directory's drive or UNC
    share (`\\server\share`). A
    drive-relative path (`E:dir`) resolves against the working directory on
    the same drive and against that drive's root otherwise
    (`resolvePathAgainst`).
  - Project files: the known project candidates are `<cwd>/.omp/config.yml`,
    `<cwd>/.omp/settings.json`, `<cwd>/.claude/settings.json`, and
    `<cwd>/.claude/settings.yml`. They are always fingerprinted individually.
    Upstream project settings providers are open-ended, so every other regular
    file directly inside `<cwd>/.omp` and `<cwd>/.claude` is also
    fingerprinted, up to 256 entries per directory. Above that limit, the
    directory's modification time records file creation and removal, and the
    known candidates remain individually fingerprinted. Directories are
    opened with `openFingerprintDir`. On Unix it uses
    `O_DIRECTORY|O_NONBLOCK`, so a FIFO or other non-directory is rejected
    without blocking.
- Installation ownership: `<dir(dir(realpath(executable)))>/manager.json`,
  which is the manifest that `managedInstallName` reads before any lookup,
  and bun's `<launcherDir>/<name>.bunx` ownership marker (upstream
  `bunShimMarkerPath`). The resolver's `Launcher` is the PATH-resolved path
  before symlinks are evaluated. The cache key includes both `Launcher` and
  the real `Executable`.
- Release feed: `$npm_config_userconfig` or `$HOME/.npmrc`,
  `$XDG_CONFIG_HOME/.bunfig.toml`, and `$HOME/.bunfig.toml`.
  `npm_config_userconfig` is anchored like an overlay but not lexically
  cleaned. OMP reads it with `fs.readFile`, so the filesystem resolves
  symlinks before `..`, and the fingerprint must stat that same file.
- Startup dotenv files: `$HOME/.env`, `<root>/.env` (the active profile
  config root), `<base>/.env`, `<agentDir>/.env`, and every regular file in
  `<cwd>` named `.env` or starting with `.env.`. `$HOME/.env`, `<root>/.env`,
  and `<agentDir>/.env` are the files `env.ts` applies before
  `refreshDirsFromEnv()`. `<base>/.env` is also fingerprinted, which is
  harmless when it equals `<root>/.env`. The `<cwd>` files cover upstream's
  project `.env` and Bun's dotenv autoload.

## Fingerprint

For every candidate file, the fingerprint records whether it exists, its size,
and its modification time. Creating or removing a candidate, or changing its
size or modification time, changes the key. Kandev never reads file contents,
because `.npmrc` and `.env` files can hold credentials. Normal editors and
atomic writes update the modification time.

## Bounded staleness

These inputs take effect no later than when the cache entry expires
(AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2):

- Edits to configuration files at a location chosen only through a dotenv
  value.
- Environment values interpolated into `.npmrc`.
- Output of the package-manager probes upstream runs to classify the install
  method (`bun pm bin -g`, `npm prefix -g`, `brew --prefix`, and
  `mise bin-paths`).
- Inputs that upstream adds after the pinned commit.
- An edit to a declared file that keeps both its size and its modification
  time.
