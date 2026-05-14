# Homebrew Core Submission Checklist

Step-by-step for submitting `kandev` to `Homebrew/homebrew-core`. Reference formula lives at `Formula/kandev.rb` in this repo (source-build variant, distinct from the binary-install formula at `kdlbs/homebrew-kandev`).

## 0. Prerequisites

- macOS box (arm64 preferred; ideally test on x86_64 too).
- Working `brew` install, `brew update` recent.
- `gh` CLI authenticated.
- The release whose tag is referenced in `Formula/kandev.rb` is **published** on GitHub (the source tarball URL must be resolvable).

## 1. Refresh sha256 for the pinned tag

```bash
TAG=v0.41.0   # whatever tag the formula points at
curl -sL -o /tmp/kandev-src.tar.gz "https://github.com/kdlbs/kandev/archive/refs/tags/${TAG}.tar.gz"
shasum -a 256 /tmp/kandev-src.tar.gz
```

Paste the digest into `Formula/kandev.rb` if it differs.

## 2. Local install (build from source)

```bash
cd <kandev repo root>
brew install --formula --build-from-source ./Formula/kandev.rb
```

Expected:
- `pnpm` and `go` install as build deps.
- `pnpm install --frozen-lockfile` succeeds (network during install is allowed).
- `pnpm -C apps --filter @kandev/web build` produces `apps/web/.next/standalone`.
- `bash scripts/release/package-web.sh` assembles `dist/web/`.
- `bash scripts/release/package-cli.sh` assembles `dist/kandev/cli/{bin,dist,package.json}`.
- `go build ./cmd/kandev` and `./cmd/agentctl` produce binaries with cgo + sqlite linked.
- Final layout under `$(brew --cellar kandev)/<version>/libexec/{bin,web,cli}` and `bin/kandev` wrapper script.

## 3. Smoke checks

```bash
kandev --help          # → "kandev launcher" header
kandev --version       # → 0.41.0 (or whatever tag)
which kandev           # → $HOMEBREW_PREFIX/bin/kandev
otool -L $(brew --cellar kandev)/*/libexec/bin/kandev   # cgo libs sane
```

## 4. Formula audit suite

```bash
brew test ./Formula/kandev.rb
brew style ./Formula/kandev.rb
brew audit --strict --new --online --formula ./Formula/kandev.rb
brew livecheck ./Formula/kandev.rb
```

Address every finding before submission. Common warnings to watch for:

- `FormulaAudit/Lines` — long lines or trailing whitespace.
- `FormulaAudit/Licenses` — `license` must be valid SPDX.
- `FormulaAudit/Test` — test must run an actual binary, not just `--help` against a static string. The `--version` regex assertion satisfies this.
- `FormulaAudit/Urls` — must use the `github.com` archive URL we use.
- `FormulaAudit/Components` — block ordering, no blank lines between sections.
- `Cask::Audit::Stanzas` — n/a (this is a formula, not a cask).

## 5. Reproducibility

```bash
brew uninstall kandev
brew install --formula --build-from-source ./Formula/kandev.rb
brew test kandev
```

Then on a fresh box (or `--force-bottle`-free path):

```bash
brew untap kdlbs/kandev   # avoid local tap shadowing
brew install --build-from-source ./Formula/kandev.rb
```

## 6. (Optional) HEAD install

```bash
brew install --HEAD ./Formula/kandev.rb
```

If `head do` block is present, this clones `main` and builds. Useful for catching breakage early but not required for submission.

## 7. Open the upstream PR

```bash
# 1. Fork & clone homebrew-core
gh repo fork Homebrew/homebrew-core --clone --remote
cd homebrew-core

# 2. Drop formula at sharded path
mkdir -p Formula/k
cp <kandev repo>/Formula/kandev.rb Formula/k/kandev.rb

# 3. Branch + commit
git checkout -b kandev-new-formula
git add Formula/k/kandev.rb
git commit -m "kandev 0.41.0 (new formula)"
git push origin kandev-new-formula

# 4. Open PR
gh pr create \
  --repo Homebrew/homebrew-core \
  --title "kandev 0.41.0 (new formula)" \
  --body "$(cat <<'EOF'
Adds `kandev`, an AI Kanban & development environment.

- Homepage: https://github.com/kdlbs/kandev
- License: AGPL-3.0-only
- Source build: Go (cgo) backend + Next.js standalone web bundle + TypeScript CLI esbuild bundle.
- Runtime dependency: `node`. Build dependencies: `go`, `pnpm`.

Local tests (macOS arm64):
- [x] brew install --build-from-source
- [x] brew test
- [x] brew audit --strict --new --online
- [x] brew style

Companion tap (binary-install fast path) lives at `kdlbs/homebrew-kandev`; this homebrew-core formula is a parallel source-built distribution.
EOF
)"
```

## 8. Respond to review

- Bottle CI runs automatically; failures appear as PR check comments.
- Maintainer may ask for: tighter test block, dropping `pnpm` for `npm` (lockfile compatibility), license format, or resource-vendoring of JS deps.
- Iterate until merged.

## 9. After merge

- Auto-bump worker (`BrewTestBot`) will open bump PRs on new tags via `livecheck`.
- For manual bump: `brew bump-formula-pr --version=X.Y.Z kandev`.
- Do **not** drop the `kdlbs/homebrew-kandev` tap immediately — keep it as the fast (binary) install path for some releases and announce the migration.
