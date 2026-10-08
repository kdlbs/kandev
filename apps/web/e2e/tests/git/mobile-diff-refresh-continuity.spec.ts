import { test, expect } from "../../fixtures/test-base";
import {
  createStandardProfile,
  GitHelper,
  makeGitEnv,
  openTaskSession,
} from "../../helpers/git-helper";
import { createGitEnrichmentGate, routeGitStatusRefresh } from "./git-status-refresh-helpers";
import {
  holdRefreshDuringEdit,
  isSameDiffViewerNode,
  openAllChangesDiff,
  rememberDiffViewerNode,
  releaseRefreshEnrichment,
  scrollDiffIntoReadingPosition,
  setDiffRenderer,
  triggerForegroundRefresh,
  visibleDiffAnchor,
  type DiffRenderer,
} from "./git-refresh-continuity-helpers";
import path from "node:path";

const TARGET_PATH = "z-mobile-refresh-target.txt";
const PREFIX_PATH = "a-mobile-refresh-prefix.txt";
const UNRELATED_PATH = "m-mobile-refresh-unrelated.txt";
const INITIAL_MARKER = "initial-mobile-target-line-70";
const UPDATED_MARKER = "updated-mobile-target-line-77";

function targetContent(targetMarker: string, shifted = false) {
  const lines = Array.from({ length: 140 }, (_, index) => `stable-mobile-line-${index + 1}`);
  if (shifted) {
    lines.splice(24, 5);
    lines.splice(
      30,
      0,
      ...Array.from({ length: 12 }, (_, index) => `inserted-before-anchor-${index + 1}`),
    );
  }
  lines[shifted ? 76 : 69] = targetMarker;
  return `${lines.join("\n")}\n`;
}

function prefixContent(lineCount: number, prefix: string) {
  return `${Array.from({ length: lineCount }, (_, index) => `${prefix}-line-${index + 1}`).join("\n")}\n`;
}

async function expectDiffText(
  testPage: import("@playwright/test").Page,
  renderer: DiffRenderer,
  filePath: string,
  text: string,
  present: boolean,
) {
  await expect
    .poll(
      () =>
        testPage.evaluate(
          ({ path: selectedPath, activeRenderer, expected }) => {
            const section = document.querySelector<HTMLElement>(
              `[data-review-file-key="${encodeURIComponent(selectedPath)}"]`,
            );
            if (activeRenderer === "monaco") {
              const host = window as Window & {
                monaco?: {
                  editor: {
                    getEditors: () => Array<{
                      getDomNode: () => HTMLElement | null;
                      getModel: () => { getValue: () => string } | null;
                    }>;
                  };
                };
              };
              return (host.monaco?.editor.getEditors() ?? [])
                .filter((editor) => section?.contains(editor.getDomNode()))
                .some((editor) => editor.getModel()?.getValue().includes(expected));
            }
            let content = "";
            content = section?.querySelector("diffs-container")?.shadowRoot?.textContent ?? "";
            return content.includes(expected);
          },
          { path: filePath, activeRenderer: renderer, expected: text },
        ),
      { timeout: 30_000, message: `mobile diff should ${present ? "show" : "hide"} ${text}` },
    )
    .toBe(present);
}

function expectSameAnchor(
  before: { line: number; side: string; content: string; offset: number } | null,
  after: { line: number; side: string; content: string; offset: number } | null,
) {
  expect(before).not.toBeNull();
  expect(after).not.toBeNull();
  expect(after!.content).toBe(before!.content);
  expect(after!.side).toBe(before!.side);
  expect(Math.abs(after!.offset - before!.offset)).toBeLessThanOrEqual(2);
}

test.describe("phone Git diff refresh continuity", () => {
  test.describe.configure({ timeout: 120_000 });

  for (const renderer of ["pierre-diffs", "monaco"] as const) {
    test(`keeps the full-height drawer readable with ${renderer}`, async ({
      testPage,
      apiClient,
      seedData,
      backend,
      prCapture,
    }) => {
      const repositoryPath = path.join(backend.tmpDir, "repos", "e2e-repo");
      const git = new GitHelper(repositoryPath, makeGitEnv(backend.tmpDir));
      git.exec("git reset --hard HEAD");
      git.exec("git clean -fd");
      git.createFile(PREFIX_PATH, prefixContent(16, "mobile-prefix-before"));
      git.createFile(TARGET_PATH, targetContent(INITIAL_MARKER));

      await setDiffRenderer(testPage, renderer);
      const bridge = await routeGitStatusRefresh(testPage, { dropPendingStatusEvents: false });
      const gate = createGitEnrichmentGate(backend.tmpDir);
      const title = `Mobile Diff Refresh Continuity ${renderer}`;

      try {
        const profile = await createStandardProfile(apiClient, `${title} Profile`);
        const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
          description: "e2e:delay(120000)",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        });
        if (!task.session_id) throw new Error("The seeded task should have a session");
        const priorFreshResponses = bridge.responseCount("fresh");
        const session = await openTaskSession(testPage, title);
        await expect(session.agentStatus()).toBeVisible({ timeout: 30_000 });
        await testPage.getByRole("button", { name: /Changes$/ }).tap();
        const changes = testPage.getByTestId("mobile-changes-panel");
        await expect(changes).toBeVisible({ timeout: 30_000 });
        const targetRow = changes.getByTestId(`file-row-${TARGET_PATH}`);
        await expect(targetRow).toBeVisible({ timeout: 30_000 });
        await expect(targetRow.getByText("+140", { exact: true })).toBeVisible();
        await bridge.waitForResponse("fresh", priorFreshResponses);
        await openAllChangesDiff(changes, testPage);

        const sheet = testPage.getByTestId("mobile-diff-sheet");
        await expect(sheet).toBeVisible();
        await expect(sheet.getByTestId("mobile-diff-sheet-close")).toBeVisible();
        const viewportHeight = testPage.viewportSize()?.height ?? 0;
        expect(viewportHeight).toBeGreaterThan(0);
        await expect
          .poll(async () => (await sheet.boundingBox())?.height ?? 0)
          .toBeGreaterThanOrEqual(viewportHeight * 0.95);
        expect(
          await testPage.evaluate(
            () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
          ),
        ).toBe(false);

        const diffRoot = sheet.getByTestId("review-diff-scroll");
        const targetSection = diffRoot.locator(
          `[data-review-file-key="${encodeURIComponent(TARGET_PATH)}"]`,
        );
        await expect(targetSection).toBeVisible({ timeout: 30_000 });
        await scrollDiffIntoReadingPosition(testPage, renderer, TARGET_PATH, "touch");
        const anchorBefore = await visibleDiffAnchor(testPage, renderer, TARGET_PATH);
        expect(anchorBefore?.line).toBeGreaterThan(15);
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        await rememberDiffViewerNode(testPage, renderer, TARGET_PATH);
        await prCapture.screenshot(`git-diff-continuity-mobile-${renderer}-ready`, {
          caption: `The phone full-height drawer shows a long ${renderer} diff at its reading position`,
        });

        const firstRefresh = await holdRefreshDuringEdit(testPage, bridge, gate, TARGET_PATH, () =>
          git.createFile(UNRELATED_PATH, "unrelated mobile change\n"),
        );
        const retainedStatus = targetSection.getByTestId("review-header-refresh-status");
        await expect(retainedStatus).toContainText("Diff is loading");
        await expect(targetRow.getByText("+140", { exact: true })).toBeAttached();
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        expectSameAnchor(anchorBefore, await visibleDiffAnchor(testPage, renderer, TARGET_PATH));
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        await prCapture.screenshot(`git-diff-continuity-mobile-${renderer}-pending`, {
          caption:
            "The phone drawer keeps the old patch and its internal scroll while Git enrichment is held",
        });

        await releaseRefreshEnrichment(bridge, gate, firstRefresh.priorReadyNotifications);
        await expect(retainedStatus).toHaveCount(0);
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        expectSameAnchor(anchorBefore, await visibleDiffAnchor(testPage, renderer, TARGET_PATH));

        const secondRefresh = await holdRefreshDuringEdit(
          testPage,
          bridge,
          gate,
          TARGET_PATH,
          () => {
            git.createFile(PREFIX_PATH, prefixContent(30, "mobile-prefix-expanded"));
            git.createFile(TARGET_PATH, targetContent(UPDATED_MARKER, true));
          },
        );
        await expect(retainedStatus).toContainText("Diff is loading");
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, false);
        await expect(targetRow.getByText("+140", { exact: true })).toBeAttached();

        await releaseRefreshEnrichment(bridge, gate, secondRefresh.priorReadyNotifications);
        await expect(retainedStatus).toHaveCount(0);
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, true);
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, false);
        await expect(targetRow.getByText("+147", { exact: true })).toBeAttached();
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        const changedAnchor = await visibleDiffAnchor(testPage, renderer, TARGET_PATH);
        expectSameAnchor(anchorBefore, changedAnchor);
        expect(changedAnchor?.line).not.toBe(anchorBefore?.line);
        await prCapture.screenshot(`git-diff-continuity-mobile-${renderer}-changed`, {
          caption:
            "The phone drawer restores the same line after the diff and a preceding section change",
        });

        const { sessions } = await apiClient.listTaskSessions(task.id);
        const environmentId = sessions[0]?.task_environment_id;
        if (!environmentId) throw new Error("The task session should have an environment identity");
        bridge.setFailureEnvironmentId(environmentId);
        bridge.forceFailures(["fresh", "recover"]);
        const priorFailureResponses = bridge.responseCount("fresh");
        const priorRecoveryResponses = bridge.responseCount("recover");
        await triggerForegroundRefresh(testPage);
        const failure = await bridge.waitForResponse("fresh", priorFailureResponses);
        const recovery = await bridge.waitForResponse("recover", priorRecoveryResponses);
        expect(failure.success).toBe(false);
        expect(recovery.success).toBe(false);
        await expect(retainedStatus).toContainText("Diff is unavailable");
        await expect(targetRow.getByText("+147", { exact: true })).toBeAttached();
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, true);
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        bridge.allowResponses();

        git.exec(`rm ${TARGET_PATH}`);
        await triggerForegroundRefresh(testPage);
        await expect(targetSection).toHaveCount(0, { timeout: 30_000 });
        await expect(targetRow).toHaveCount(0, { timeout: 30_000 });

        await sheet.getByTestId("mobile-diff-sheet-close").tap();
        await expect(sheet).toBeHidden();
        await expect(changes).toBeVisible();
        expect(
          await testPage.evaluate(
            () =>
              !document
                .querySelector("[data-testid='mobile-diff-sheet']")
                ?.contains(document.activeElement),
          ),
        ).toBe(true);
        await prCapture.screenshot(`git-diff-continuity-mobile-${renderer}-dismissed`, {
          caption: "The phone diff drawer dismisses cleanly after confirmed file removal",
        });
      } finally {
        gate.dispose();
      }
    });
  }
});
