---
id: "01-route-file-actions"
title: "Route external file actions to their repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EXTERNAL-FILE-TARGET-001
acceptance_criteria:
  - AC-UI-EXTERNAL-FILE-TARGET-001.1
  - AC-UI-EXTERNAL-FILE-TARGET-001.2
  - AC-UI-EXTERNAL-FILE-TARGET-001.3
  - AC-UI-EXTERNAL-FILE-TARGET-001.4
  - AC-UI-EXTERNAL-FILE-TARGET-001.5
system_design:
  - ../../specs/ui/system-design/repository-scoped-external-file-launch.md
---

# Task 01: Route external file actions to their repository

## Summary

Prove the current secondary-file misroute at the rendered Review menu and HTTP-domain boundary, then route external file actions using the correct session worktree. Preserve the existing legacy default only where the design permits it and disable unresolvable targets.

## In scope

- Review desktop and phone, Monaco desktop/overflow, CodeMirror overflow, shared file actions and session-domain target resolution.
- Independently authored real-caller regression and focused identity/compatibility tests; the editor paragraph in public developer-tools documentation.

## Out of scope

File-tree actions, folder and copy-path routing, embedded editor dispatch, code-server, native CLI line handling, SSH URL escaping, security/containment, attachments, recovery, caches, runtime/worktree inventory, settings, copy and layout changes.

## Acceptance

1. Before production changes, actual rendered Review toolbar -> actual menu -> real opener and `useRequest` -> stubbed HTTP-domain regression collects and fails solely on the secondary worktree ID; identical primary control passes. Capture commands, project, counts, failure and exit; fixture/setup failures are not RED.
2. Desktop/phone Review send the selected worktree and relative path for same-path siblings using a unique file-specific ID even without `workspace_path`; actual editor consumers resolve named scopes from observed placement. Name-only ambiguity, missing identity, independently proven conflicts and unscoped multi-worktree cases cannot launch. Legacy/single-repository cases and unrelated menu behavior follow the design.
3. Focused new and existing tests, scoped lint/type checks, staged-change i18n ratchets, public docs and spec/diff checks pass with actual counts and results recorded. State HappyDOM/native and Chromium limits accurately.

## UI-01: Existing file actions

[Full preview](plan.md#ui-01-existing-file-actions). AC 001.1-001.5.

```text
Desktop Review: Open with -> VS Code [enabled if resolved]
Phone Review: More actions -> VS Code [disabled if unknown]
```

Use existing menus, labels and accessible disabled state. No composition, touch geometry or scrolling change; verify actual phone overflow in the rendered boundary test.

## Verification

Only after the separate implementation request and exclusive heavy grant: prepend the pinned pnpm 9.15.9 directory `/home/jcfs/.local/share/mise/installs/pnpm/9.15.9`, Node 24.21.0 `bin`, and Go 1.26.0 `bin` to PATH; confirm versions. Run `pnpm install --frozen-lockfile` once in `apps/` if needed. Run heavy operations sequentially and join every actual native handle. Each `scripts/run-quiet` invocation gets a unique task-owned `KANDEV_RUN_QUIET_DIR`; do not use `--summary` for these unit checks. Obtain a fresh typecheck memory grant before typecheck. Resolve ordinary fixture/lint corrections at the approved cap; pause for unknown, auth, resource/OOM or out-of-scope failures.

Author `components/review/review-diff-toolbar.external-editor.test.tsx` first. Its test names must include `secondary repository routes the selected file` and `primary repository control`. Before production changes run:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales components/review/review-diff-toolbar.external-editor.test.tsx --maxWorkers=1 --no-file-parallelism)
```

Assert secondary failure is the missing `worktree_id`. The primary control accepts an omitted ID because the known first worktree is the backend default, or an explicit primary ID after the fix; it does not execute the backend. Do not count fixture setup errors as RED. Then implement and run the following after creating the two additional test files named below:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales components/review/review-diff-toolbar.external-editor.test.tsx components/editors/file-actions-repository-target.test.tsx hooks/domains/session/use-external-file-target.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec vitest run --project browser components/review/review-diff-toolbar.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx hooks/use-open-session-in-editor.test.ts hooks/use-open-session-in-editor.wire.test.tsx --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec eslint --max-warnings 0 <every-actual-changed-ts-and-tsx-file-including-tests>)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Replace the lint placeholder with the complete actual owned TS/TSX file list, including helpers and tests, and record it. Stage actual owned additions before both staged-change ratchets and current actual PR-documentation coverage preflight; use normal hooks and no bypass. Check the actual PR-docs preflight interface before running it and record its result. Do not stage at the design checkpoint. Confirm test discovery, project and nonzero counts; a green empty selection is invalid. Update requirement/design status only after implementation matches them, and record all actual results here and in the plan.

The new Review fixture mounts `StateProvider`, store, `TooltipProvider`, real dropdown, opener and request; stub only HTTP-domain `openSessionInEditor` and unrelated display dependencies. Use two explicit associations with same `src/shared.ts`, distinct worktree and repository IDs and paths, a legacy primary `worktree_path`, no `workspace_path`, and click the actual VS Code menuitem. Add explicit Review ID without name and relevant legacy-primary controls. Add phone overflow with identical secondary assertion. The editor consumer suite renders actual Monaco dropdown and overflow, and CodeMirror overflow; assert HTTP payload and guard against the session-primary ID overriding a named secondary. Cover same-path, ID with absent or unresolvable redundant name, name-only placement, independently proven ID/name conflict, duplicate/missing association or placement, unknown and nested name-only scope, missing session, single/legacy/checkout fallback, multiple unscoped worktrees, external vs embedded item, folder and copy controls. Reuse `workspaceRelativeFilePath` and `isWorkspaceTreePath` where their contracts fit; do not use `getSessionWorkspacePath` legacy worktree fallback as the root for named placement or add path aliases. Tests must assert observable requests or disabled state, not copy implementation logic.

## Bounded legacy review follow-up: complete

The initial work remains complete. A separately authorized follow-up must preserve the existing omitted-ID default only when the file is unscoped, its nonempty ID equals `session.repository_id`, API `session.worktrees` is absent or empty, the known worktree list has exactly one entry with a usable nonempty ID and no repository ID, and that worktree's ID equals nonempty `session.worktree_id`. Return `{}` so the real opener omits `worktree_id`. Any different known worktree ID, nonempty conflicting worktree repository ID, multiple known worktrees, named scope, unknown file ID, or API association prevents this special fallback. Other unique association behavior remains as designed. Do not introduce broad defaulting, inventory recovery, backend changes, or new UI copy.

Before production edits, independently add a test to the existing `components/editors/file-actions-repository-target.test.tsx` that renders actual `CodeMirrorToolbar` overflow with a session primary repository ID, no file scope, no API associations, legacy `worktree_id`, and the one WS worktree without repository ID. Open the actual menu and assert the external item is enabled, then click it and assert the real opener and `useRequest` reach the HTTP-domain stub with the original file path and omitted `worktree_id`. Current code must fail specifically because the actual item is disabled; record native handle, exit and nonzero collection. Add focused resolver cases for the exact positive predicate and every stated negative control; do not accept an explicit worktree ID as the oracle because the server can lack the association.

After a separate implementation request and exclusive HEAVY grant, use the already installed pinned tools and run sequentially:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales components/editors/file-actions-repository-target.test.tsx -t 'legacy single worktree' --maxWorkers=1 --no-file-parallelism)
# Record the causal RED before production changes.
(cd apps/web && pnpm exec vitest run --project browser-locales components/editors/file-actions-repository-target.test.tsx hooks/domains/session/use-external-file-target.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/editors/file-actions-repository-target.test.tsx hooks/domains/session/use-external-file-target.ts hooks/domains/session/use-external-file-target.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
```

Confirm actual changed TS files before lint; include any additional changed owned test/helper. Refresh memory before typecheck, follow the approved cap and resource/error checkpoint, stage actual additions before applicable ratchets and current documentation-coverage evaluation, and run normal hooks without bypass. Record counts, exits and actual joined handles. Run only checks affected by the correction, not broad passing-suite replays. No new screenshot or capture build is needed for the unchanged menu composition and pure target-data case.

Results: rendered CodeMirror RED collected one selected test and failed on the disabled external item; the corrected real opener reached the HTTP-domain boundary with omitted worktree ID. The two affected suites passed 12 tests, scoped ESLint passed on the three changed TS files, and the 6144 MiB canonical typecheck passed. Current staged ratchet, documentation and normal-hook results are recorded in the delivery checkpoint.

## Required CI remediation: complete

E2E Shard 23/28 on synthetic merge `89d32b81112ed0dc572543a9d6dfd3dba229ac63` (base `67c25d4a136f2e80a50f8b3753ff7bb2471da6bb`, head `e458236dd8b07377daea8a9f240ccc2e5b851e72`) failed the update-notification test in all four CI attempts with `browserContext.addCookies: Invalid URL`. The actual retry trace records the cookie URL as the relative `/settings/preferences/appearance`. Its embedded test source is byte-identical to that base file (SHA-256 `3308715d1c81c2746c23ad31ead9b304d9c57f0ca38135c4b90c738fc688e41d`): the helper takes `(page, url, frontendUrl)` while that one caller passes `(page, backend.frontendUrl, screen.url)`. This is a test argument-contract mismatch in the actual base checkout, not a native-editor routing failure or a proven infrastructure flake. CI evidence: run `40610650072`, job `122570054581`, 50 passed and one failed in its process.

The following is the **complete** affected-file diff from the frozen candidate file to base `67c25d4`, before the correction. Blank context-line prefixes are omitted for whitespace hygiene. It is shown to prevent applying the new argument order against the candidate's old helper by itself:

```diff
diff --git a/candidate-pseudo-coverage.spec.ts b/base-67c25d4-pseudo-coverage.spec.ts
index 881c69395e..fad1fc5d07 100644
--- a/candidate-pseudo-coverage.spec.ts
+++ b/base-67c25d4-pseudo-coverage.spec.ts
@@ -498,11 +498,17 @@ const ALLOWED = [
  */
 const ACCENTED = /[À-ɏ]/;

-async function activatePseudo(page: Page, frontendUrl: string, url: string) {
-  // Seed the locale before the first document request. The Go shell uses this
-  // cookie to set both <html lang> and the boot payload.
-  await page.context().addCookies([{ name: "kandev_locale", value: "pseudo", url: frontendUrl }]);
-  await page.goto(new URL(url, frontendUrl).href);
+async function activatePseudo(page: Page, url: string, frontendUrl: string) {
+  // Seed the locale before the first document request: the Go shell reads this
+  // cookie to set <html lang> and the boot payload.
+  await page
+    .context()
+    .addCookies([{ name: "kandev_locale", value: "pseudo", url: frontendUrl, sameSite: "Lax" }]);
+  const initialResponse = await page.goto(url);
+  if (!initialResponse) throw new Error("Pseudo-locale navigation returned no response");
+  expect(initialResponse.ok()).toBe(true);
+  const initialHeaders = await initialResponse.request().allHeaders();
+  expect(initialHeaders.cookie ?? "").toMatch(/(?:^|;\s*)kandev_locale=pseudo(?:;|$)/);
   // `<html lang>` proves the SHELL is on the pseudo locale and nothing more: the
   // Go shell writes it from the cookie, so it is already correct in the HTML the
   // browser parses, before any script has run. Every stronger fact — the route
@@ -874,7 +880,7 @@ test.describe("i18n pseudo-locale coverage", () => {
       if (!screen) throw new Error("The secrets screen is missing");
       const secret = await apiClient.createSecret(name, "synthetic-pseudo-test-value");
       try {
-        await activatePseudo(testPage, backend.frontendUrl, screen.url);
+        await activatePseudo(testPage, screen.url, backend.frontendUrl);
         await waitForScreen(testPage, screen);
         await waitForTransientLoadingToFinish(testPage);
         const row = testPage.getByTestId(`secret-row-${secret.id}`);
@@ -916,7 +922,7 @@ test.describe("i18n pseudo-locale coverage", () => {

   for (const screen of SCREENS) {
     test(`no un-externalized copy on ${screen.name}`, async ({ testPage, apiClient, backend }) => {
-      await activatePseudo(testPage, backend.frontendUrl, screen.url);
+      await activatePseudo(testPage, screen.url, backend.frontendUrl);
       await waitForScreen(testPage, screen);
       await waitForTransientLoadingToFinish(testPage);

@@ -976,7 +982,7 @@ test.describe("i18n pseudo-locale coverage", () => {
     const appearance = SCREENS.find((screen) => screen.url === "/settings/preferences/appearance");
     if (!appearance) throw new Error("the appearance screen is no longer in SCREENS");

-    await activatePseudo(testPage, backend.frontendUrl, appearance.url);
+    await activatePseudo(testPage, appearance.url, backend.frontendUrl);
     await waitForScreen(testPage, appearance);
     await waitForTransientLoadingToFinish(testPage);

```

After separate authorization and exclusive HEAVY, reconcile only `apps/web/e2e/tests/i18n/pseudo-coverage.spec.ts` to the actual base file above, preserving its helper, first-request cookie assertions and already-updated callers. Against that base-shaped file, make the sole missed notification call:

```diff
-    await activatePseudo(testPage, backend.frontendUrl, screen.url);
+    await activatePseudo(testPage, new URL(screen.url, backend.frontendUrl).href, backend.frontendUrl);
```

The absolute appearance URL is the navigation argument; `backend.frontendUrl` is the cookie URL. Before using the pinned base source, check the current target base and the file's exact content; if it changed, stop and reconcile with ROOT rather than overwriting newer work. No whole-branch rebase, generic i18n/parser changes, helper refactor, new companion tests, or unrelated correction is authorized by this work order.

Run a fresh managed build and exactly the failed Chromium case, one worker and zero retries, then its E2E-file lint and sleep ratchet. The earlier capture build predates the current production correction and is not a valid `--no-build` basis. From the repository root with the previously pinned toolchain on PATH:

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --grep 'localizes a Kandev update notification received from the backend' --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/tests/i18n/pseudo-coverage.spec.ts)
```

Verify one test actually collected and passed, retain and join each native handle, and record exits, counts, fixture cleanup and the exact checked source. Stage the owned test and this work order before the E2E sleep ratchet, current full-PR documentation coverage, catalog/spec/diff checks and normal hooks without bypass. The test lives under `e2e`, which `apps/web/tsconfig.json` explicitly excludes; this correction changes no typed production code, so do not repeat the full web typecheck already passed for the actual product correction. Preserve the earlier capture assets and evidence limits. No CI retry or observer renewal follows automatically.

Result: reconciled the test file to base `67c25d4` and changed only the notification call against that base. A fresh managed host Chromium run collected the exact case with one worker and zero retries and passed it (1 passed); the cookie, navigation and localized-notification assertions executed. Scoped E2E ESLint passed. The normal Prettier hook reformatted the shared call and rejected the first commit; a `prettier-ignore` directive preserves the explicitly required identical call line for the parallel correction. The two existing PR screenshots and manifest were backed up before the runner and restored with matching SHA-256 hashes. No web typecheck or broad suite replay was performed.

## Files likely touched

- `apps/web/components/review/review-diff-toolbar.tsx` and new rendered test.
- `apps/web/components/editors/file-actions-dropdown.tsx`, `editor-toolbar-overflow.tsx`, `monaco/monaco-editor-toolbar.tsx`, `codemirror/codemirror-editor-toolbar.tsx`, and new consumer test.
- `apps/web/hooks/domains/session/` target hook/resolver and tests; reuse `use-session-worktrees.ts` and existing session selectors.
- `docs/public/developer-tools.md`, this work package and paired specifications.

## Dependencies

None. Separate implementation authorization and exclusive heavy grant are prerequisites to execution, not work-order dependencies.

## Risks

Name is a workspace scope, not a display name. The editor panel's repository ID denotes the session primary, not necessarily the open file. Unknown placement or nested scope without an authoritative file-specific ID intentionally disables launch. Backend membership rejection remains authoritative. HappyDOM does not prove native or Chromium behavior.

## Parallelism

Sequential.

## Inputs

Paired requirement and system design, `use-session-worktrees.ts`, `file-tree-editor-target.ts` as a contrasting path shape, `service.go` session resolution, existing toolbar and opener tests, and `docs/public/developer-tools.md`.

## Results

- Independently authored Review RED: browser-locales, 2 collected; secondary failed on omitted worktree ID and primary passed. Shared overflow follow-up RED: the external item was disabled when the raw primary ID accompanied the secondary name.
- Final browser-locales focused suites: 3 files, 14 tests passed; after the fixture-only type correction, the changed editor suite passed 6 tests. Existing browser suites: 4 files, 43 tests passed. Both environments use HappyDOM.
- Scoped ESLint passed for all eight changed TS/TSX files. Web typecheck passed at 6144 MiB after correcting required test fixture props. i18n check and ratchet, E2E sleep ratchet, staged PR documentation coverage, catalog/spec lint, public docs tests (63 passed), public docs validation (54 pages), and staged diff check passed.
- Managed one-worker mobile-chrome disposable capture passed after correcting fixture navigation: desktop-width and phone-width Review menus were captured. The fixture was removed. Screenshots show menu presentation; no native editor launch or multi-repository Chromium routing is claimed.
