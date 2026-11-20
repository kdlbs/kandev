---
id: "01-natural-file-order"
title: "Natural file order"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-TREE-NATURAL-ORDER-001
acceptance_criteria:
  - AC-UI-FILE-TREE-NATURAL-ORDER-001.1
  - AC-UI-FILE-TREE-NATURAL-ORDER-001.2
  - AC-UI-FILE-TREE-NATURAL-ORDER-001.3
  - AC-UI-FILE-TREE-NATURAL-ORDER-001.4
system_design:
  - ../../specs/ui/system-design/file-tree-natural-order.md
---

# Task 01: Natural File Order

## Summary

Make the shared Files-tree ordering use default-locale numeric collation while
retaining directories first and the existing lexical tie-break. Prove the
changed order through focused utility regressions and isolated real desktop and
phone interactions, then document the visible behavior.

## In scope

- Extend existing utility tests, retaining their original fixtures, assertions,
  and meaning. Run and record a causal RED before changing production code.
- Change `compareTreeNodes`; ensure roots, nested children, inserts, renames,
  and visible paths share the result without mutating source arrays.
- Add the two dedicated E2E files and the single concise public-doc instruction
  beside the Files paragraph. Capture owned synthetic screenshots where needed
  for the authorized PR workflow, preserving original files and hashes outside
  the managed asset directory before it is cleared.

## Out of scope

File/path/content/backend mutation, search sorting, hidden-file policy, sort
controls or preferences, persistence, APIs, global CSS, labels/catalogs,
number parsers, dependencies, and semantic-version/decimal/negative parsing.

## Acceptance

- Root and nested directory/file sequences use the shared numeric comparator,
  preserve lexical ties and nonnumeric behavior, and do not mutate input;
  inserts, renames, and visible DFS order agree.
- Real desktop rendered rows and keyboard range selection agree; configured
  phone rows show the same order, touch opens the intended numbered file, and
  the document has no horizontal overflow.
- Original regression coverage remains intact and the public Files paragraph
  gains only a concise natural-order instruction.

## ASCII UI preview

[Full preview and responsive notes](plan.md#ascii-ui-preview). UI-01 and UI-02
map to AC .1, .3, and .4. Row order is required; spacing is illustrative.

```text
Desktop Files           Phone Files
  folder2/                folder2/
    step2.md                step2.md <- tap opens file
    step10.md               step10.md
  folder10/               folder10/
  step2.md              [bottom navigation]
  step10.md
```

## Verification

Only after the separate execution and HEAVY releases: resolve the pinned
pnpm 9.15.9, Node 24.21.0/bin and Go 1.26.0/bin paths and set `PATH` in that
order; set `GOTOOLCHAIN=local`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`. Install once
from `apps/` with `pnpm install --frozen-lockfile` before any pnpm command or
hook. Serialize the commands below, including the 8 GiB Node heap limit for
web typecheck. Run from repository root; each `rq` invocation allocates a new
external task-owned quiet directory. Record actual native handles and join each
to exit; do not infer results from duplicate runs. Use `--summary` only with
E2E/playwright tags. The first focused test invocation is the required RED;
repeat after implementation as GREEN using a fresh quiet directory.

```bash
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
rq() { KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-natural-order-quiet.XXXXXXXX)" scripts/run-quiet "$@"; }
rq unit -- pnpm --dir apps/web exec vitest run components/task/file-tree-utils.test.ts
# Implement after recording the causal RED, then run the same command for GREEN.
rq unit -- pnpm --dir apps/web exec vitest run components/task/file-tree-utils.test.ts
rq typecheck -- env NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
rq lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/task/file-tree-utils.ts components/task/file-tree-utils.test.ts e2e/tests/task/file-tree-natural-order.spec.ts e2e/tests/task/mobile-file-tree-natural-order.spec.ts
rq i18n -- pnpm --dir apps/web run i18n:check
rq i18n-ratchet -- pnpm --dir apps/web run i18n:ratchet
rq e2e-sleep -- pnpm --dir apps/web run e2e:sleep-ratchet
rq public-docs-test -- node --test scripts/validate-public-docs.test.mjs
rq public-docs -- node scripts/validate-public-docs.mjs
rq spec-catalog -- python3 scripts/list-docs.py validate
rq spec-lint-test -- python3 scripts/lint-spec-files.test.py
rq spec-lint -- python3 scripts/lint-spec-files.py --all
rq diff -- git diff --check
rq web-build -- make build-web-e2e
rq backend-build -- make -C apps/backend build build-e2e-delivery-fixture
rq plugin-build -- make -C apps/backend e2e-plugin-ui e2e-plugin-package
rq e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium -- tests/task/file-tree-natural-order.spec.ts --workers=1 --retries=0
rq e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome -- tests/task/mobile-file-tree-natural-order.spec.ts --workers=1 --retries=0
```

Before the ratchet, stage actual new frontend paths as required for its staged
coverage; preserve concurrent work. Run own same-worktree matching builds before
`--no-build` E2E. Verify the desktop and mobile screenshot/manifest originals
and hashes externally before any managed run that clears assets. Run no full
suite, shared build, extra agent, or unowned process/cache/temp/volume cleanup.
Honor `/home/jcfs/a.txt` exclusions. Public docs and spec validation are
pending until execution; promote draft specs only after verified completion.

## Files likely touched

- `apps/web/components/task/file-tree-utils.ts`
- `apps/web/components/task/file-tree-utils.test.ts`
- `apps/web/e2e/tests/task/file-tree-natural-order.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-tree-natural-order.spec.ts`
- `docs/public/developer-tools.md`
- This plan, work order, and the paired UI requirements and design.

## Dependencies

None. ROOT must fully read the four artifacts and issue a later separate
execution request and exclusive HEAVY release before execution.

## Risks

- A numeric tie can expose input order without the lexical fallback; assert both
  input orders. Locale collation may still equate distinct strings lexically.
- Existing PR 1512 modifies the same helper file outside the comparator; preserve
  its `mergeTreeNodes` change and reassess if its head changes. Re-inventory all
  actual owned paths and main/source gap before publication.

## Parallelism

`sequential`; no native delegates or extra sessions.

## Inputs

- [Requirements](../../specs/ui/requirements/file-tree-natural-order.md)
- [System design](../../specs/ui/system-design/file-tree-natural-order.md)
- Existing `file-tree-utils.test.ts`, `file-tree-sorting.spec.ts`, and
  `mobile-file-tree-path-copy.spec.ts` patterns.

## Results

- Causal RED: the focused utility suite failed on root numeric ordering and
  visible-path order before the comparator change (native handle 15147, exit 1).
- GREEN: `pnpm --dir apps/web exec vitest run components/task/file-tree-utils.test.ts`
  passed all 17 tests; after strengthening the rename and lexical tie checks,
  the same suite and focused test lint passed again.
- The work-order typecheck (8 GiB Node heap), targeted ESLint, i18n check and
  ratchet, E2E sleep ratchet, public-doc validators, spec catalog and linters,
  and diff check passed.
- `make build-web-e2e`, `make -C apps/backend build build-e2e-delivery-fixture`,
  and `make -C apps/backend e2e-plugin-ui e2e-plugin-package` passed in this
  worktree before the browser tests.
- The exact desktop Chromium and mobile-chrome E2E commands above each passed
  one test with one worker and zero retries. Desktop selection and phone
  opening used real keyboard and touch input. Both synthetic screenshots and
  manifests were preserved externally with verified SHA-256 hashes.
