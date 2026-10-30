---
created: 2026-08-22
updated: 2026-10-30
status: complete
requirements:
  - REQ-PLUGINS-MARKETPLACE-001
  - REQ-PLUGINS-MARKETPLACE-002
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/plugins/system-design/native-plugin-attribution.md
legacy_specs: []
---

# Fix plan: Plugin marketplace attribution

## Overview

Repair [issue 4750](https://github.com/kdlbs/kandev/issues/4750) in the host.
The official native-plugin catalog attributes entries to their curated
repository owner. Installed details identify manifest authors as declared text.
Tasks 04 and 05 were implemented sequentially and are complete.

Plugins owns this package because it owns catalog attribution and manifest
presentation. This package amends the existing marketplace contract. It does
not create a separate repair requirement or a UI-owned requirement.

## Confirmed diagnosis

The investigated checkout and remote main both resolved to
`838043071d99ae3e9df22fd6ef8efa161ea01c73` on 2026-10-30.
The canonical issue had no comments or image attachments.

`buildEntry` currently assigns `presentation.author || meta.author`.
A community manifest can therefore supply `kandev` despite its curated owner.
`buildIndex` also publishes eligible prior records unchanged. A failed release
can preserve the earlier attribution while healthy peers advance.

`MarketplaceEntryRow` shows `entry.author` through `plugins:byAuthor`.
`PluginManifestCard` shows installed manifest data under the generic `Author`
label. The native repository link already follows the curated pointer because
`parseManifestFields` excludes `repo_url`. The link is a preserved invariant,
not an additional confirmed defect.

A temporary in-memory reproduction used the real `buildEntry` and `buildIndex`
with deterministic fetch and package-verifier stubs. It produced:

| Path | Expected attribution | Observed attribution |
| --- | --- | --- |
| Fresh native entry owned by Fclem | Fclem | kandev |
| Same entry with repository metadata HTTP 503 | Fclem | kandev |
| Retained old entry plus a fresh healthy peer | Fclem | kandev |

The repository-link assertion passed. The reproduction changed no files and
started no runtime. This proves catalog projection and retention behavior,
not a live browser session or the current public catalog contents. The public
index was unavailable through the web fetch tool.

Three existing targeted tests passed. One explicitly expects the spoofed
manifest name. The first permanent regression will be
`buildEntry attributes native plugins to the curated repository owner` in
`plugin-registry/build-index.test.mjs`. It must fail before the source correction.

## Requirement conformance

The previous `AC-PLUGINS-MARKETPLACE-001.4` explicitly preserved manifest-first
attribution. This is an intentional contract correction, not a violation of
that earlier criterion. The amended criterion defines curated-owner attribution.
New criteria `.9` and `.10` cover retention and installed declaration labels.
Criterion `.5` preserves canonical catalog repository links.
`AC-PLUGINS-MARKETPLACE-002.3` retains publication failure behavior.

The [native attribution design](../../specs/plugins/system-design/native-plugin-attribution.md)
contains the complete contract and trust limits. The existing central
[publication decision](../../decisions/2026-08-30-central-curated-plugin-release-polling.md)
continues to govern release admission and retention.

## Scope

### In scope

- Native catalog attribution from the reviewed registry pointer.
- Attribution correction on eligible retained native records.
- Installed manifest labels and all supported translations.
- Targeted Node, component, desktop, and phone evidence.
- Registry and public documentation updates with implementation.

### Out of scope

- Verified publisher badges, installed-file evidence, or provenance receipts.
- Changes to native installation, signatures, package digests, or permissions.
- Canvas attribution or its inspected archive contract.
- New trust enforcement for operator-added sources.
- External plugin changes, registry deployment, or PR creation by this package.

## Technical approach

Task 04 changes `buildEntry` for native entries, including omitted `kind`.
Use the owner from `spec.repo`, independently of manifest data or GitHub metadata.
Keep the existing canvas author path. Normalize only the author of eligible
retained native records after `trustedPriorRecord` accepts their identity.
Copy retained objects and preserve release data and publication guards.

Task 05 changes the existing `plugins:manifestAuthor` label to `Declared author`.
Keep `PluginRecord.author` and raw manifest output unchanged. Add component
proof and browser flows using `PluginMarketplaceReleaseFixture`. Rebuild its
index before adding the fixture source so the browser consumes real builder
output instead of a hand-authored corrected response.

The JSON schema, backend API, source precedence, and catalog cache remain
compatible. First-party native listings change from `by kandev` to `by kdlbs`.
A source-triggered rebuild, manual rebuild, or daily fallback publishes this
metadata change without requiring a plugin release. An all-failed rebuild
still leaves the old published site unchanged.

## ASCII UI preview

UI-01: Settings > Plugins > Browse, native listing after a successful rebuild.
Desktop and phone share this affected metadata region:

```text
Before: Prompt history v1.4.2
        by kandev     Repo: Fclem/kandev-plugin-prompt-history
After:  Prompt history v1.4.2
        by Fclem      Repo: Fclem/kandev-plugin-prompt-history
```

This is a data correction within the existing wrapping metadata footer.
The card retains its primary install action and existing navigation.
Maps to `AC-PLUGINS-MARKETPLACE-001.4` and `.5`.

UI-02: Settings > Plugins > Installed > plugin detail > Manifest.
Only the affected row is shown:

```text
Desktop before: Author             kandev
Desktop after:  Declared author    kandev
Phone before:   Author                   kandev
Phone after:    Declared author          kandev
Missing author: Declared author                -
```

Desktop retains the manifest grid. Phone retains direct detail navigation,
its single-column card, and the existing settings scroll container.
The nearest mobile exemplar is
`apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`.
The label identifies a declaration in both views. The original author value
remains visible and appears unchanged in raw manifest output.
Maps to `AC-PLUGINS-MARKETPLACE-001.10`.

Grouping and semantic labels are required. Spacing is illustrative.
Localized labels must remain readable without document overflow. This content
change requires no additional phone surface, control, or scroll owner.

## Tests

| Criteria | File and regression |
| --- | --- |
| `AC-PLUGINS-MARKETPLACE-001.4` | `plugin-registry/build-index.test.mjs`: `buildEntry attributes native plugins to the curated repository owner` |
| `AC-PLUGINS-MARKETPLACE-001.4`, `.5` | Same file: owner cases for first-party, missing author, failed metadata, missing metadata owner, conflicting metadata owner, and manifest repository claims |
| `AC-PLUGINS-MARKETPLACE-001.9`, `AC-PLUGINS-MARKETPLACE-002.3` | Same file: `buildIndex replaces retained native attribution without changing release evidence`; mixed healthy/failed entries, immutable prior input, and all-failed publication refusal |
| `AC-PLUGINS-MARKETPLACE-001.4` | Existing inspected-canvas test remains unchanged and passes, proving native-only scope |
| `AC-PLUGINS-MARKETPLACE-001.4`, `.5` | `apps/web/components/settings/plugins/marketplace-entry-row.test.tsx`: curated author and repository presentation |
| `AC-PLUGINS-MARKETPLACE-001.10` | New `apps/web/components/settings/plugins/plugin-manifest-card.test.tsx`: declared label, unchanged author/raw output, and missing-author marker |

## E2E tests

Task 05 owns these flows in `apps/web/e2e/tests/plugins/`:

- `plugin-attribution.spec.ts`, project `chromium`.
- `mobile-plugin-attribution.spec.ts`, project `mobile-chrome`.
- Shared scenario setup in `plugin-attribution-helpers.ts`.

Both flows use the existing local release fixture with a manifest author of
`kandev` and curated owner `acme`. Run the real builder, register its source,
and assert `by acme` with the canonical repository link. Install the package
from Browse and open installed details through visible controls. Assert
`Declared author: kandev`, raw manifest preservation, and the same result after
reload. These flows cover `.4`, `.5`, and `.10` end to end.

The phone flow uses touch navigation and confirms readable metadata within the
existing scroll container. Capture the affected Browse and Manifest regions
for desktop and phone. Keep fixture/source/plugin cleanup in `finally`.
Task 04's mixed-state Node regression owns retained-record evidence.

## Work orders

- [x] [Task 04: Derive native catalog attribution](task-04-curated-owner-attribution.md), wave 1, done.
- [x] [Task 05: Label declarations and prove rendered attribution](task-05-declared-author-presentation.md), wave 2, depends on Task 04, done.

Execution was sequential in the primary session. There is no delegation authorization.

## Historical work

The earlier package corrected individual Bitbucket and YouTrack manifests.
Its remaining release-dependent work is cancelled in this package. Historical
local results remain in the files below. This does not establish whether an
external PR merged or a release shipped, and changes no external repository.

- [Task 01](task-01-bitbucket-attribution.md): cancelled, historical manifest correction.
- [Task 02](task-02-youtrack-attribution.md): cancelled, historical manifest correction.
- [Task 03](task-03-registry-verification.md): cancelled, obsolete manifest-first catalog proof.

## Verification commands

Each active work order contains its exact product checks. Run these shared
artifact checks from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/plugin-marketplace-attribution
git status --short -- docs/plans/plugin-marketplace-attribution
```

Use the repository coverage validator for a prospective production diff.
The synthetic source path below forces reference validation during this
document-only turn. It does not modify production code:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/plugin-marketplace-attribution';
const files = fs.readdirSync(dir).filter(name => name.endsWith('.md'))
  .map(name => `${dir}/${name}`);
files.push('docs/specs/plugins/requirements/marketplace.md',
  'docs/specs/plugins/system-design/marketplace.md',
  'docs/specs/plugins/system-design/native-plugin-attribution.md');
const fileContents = Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')]));
const changedFiles = files.map(filename => ({filename, status: 'modified'}));
changedFiles.push({filename: 'plugin-registry/build-index.mjs', status: 'modified'});
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Verification results

Design turn, 2026-10-30:

- Three existing targeted Node tests passed.
- Temporary real-source reproduction exposed three expected-behavior failures.
- Canonical native repository-link assertion passed.
- Issue assignment to `carlosflorencio` succeeded.
- Specification catalog validation and full specification lint passed.
- All 36 specification-linter tests passed.
- Prospective production-path documentation coverage passed for all five work orders.
- Local Markdown links and whitespace passed for all nine package documents.
- `git diff --check` passed. Only internal package and specification files changed.
- Implementation, component, and browser verification: pending the implementation turn.

Implementation turn, 2026-10-30:

- PR 3793 was still open at `d7b76a839a85b049fc7a2eb50c6f937c113f9c4d`; remote
  main matched the investigated baseline, so the package proceeded unchanged.
- The named builder regressions failed against the pre-fix source for the
  expected owner mismatch and retained author, then passed after the correction.
- The full registry Node suite passed: 51 tests across index building, release
  detection, and workflow contracts.
- A post-implementation review correction restored the existing canvas
  metadata-owner fallback for an empty inspected author, added
  metadata-available and metadata-failure coverage, and kept the native path
  independent of manifest and metadata authors.
- The plugin settings component folder passed 18 files / 111 tests.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, ESLint on the changed files,
  and the web typecheck passed.
- Desktop and phone attribution E2E flows passed against the real rebuilt
  fixture index. Screenshots confirmed `by acme` in Browse and
  `Declared author: kandev` in installed details on both viewports.
- Public-docs validation and its 63 tests passed. Specification catalog
  validation, all 36 linter tests, and full specification lint passed.
- Documentation coverage for the production builder path passed.

Production code, permanent component tests, translations, E2E flows, and public
documentation are now changed. The package remains unstaged and uncommitted for
review.

## Risks

- [PR 3793](https://github.com/kdlbs/kandev/pull/3793) was open at
  `d7b76a839a85b049fc7a2eb50c6f937c113f9c4d`. It changes the same builder,
  labels, and contracts. Resolve its current state and remote main before
  implementation. If it lands first, reassess this package against its behavior.
- That PR snapshot uses marketplace requirement 002 for preview images, while
  current main uses it for release publication. Do not copy its stale IDs.
- Owner attribution identifies the curated repository, not a verified publisher.
  Custom-source trust, canvas attribution, and installed provenance remain limited.
- First-party native identity visibly changes to `kdlbs`. Do not restore the
  previous brand alias through a manifest fallback.
- During a provider-wide outage, an older published catalog can retain old
  attribution until publication succeeds. Preserve the all-failed build guard.
- The public Browse screenshot shows the last published index, so it keeps the
  old `by kandev` attribution until the registry workflow publishes. Refresh
  that capture after publication; do not edit it to simulate unpublished data.
