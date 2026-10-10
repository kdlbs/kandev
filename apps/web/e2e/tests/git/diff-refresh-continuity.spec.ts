import { seedContinuityPreviewHistory } from "./git-continuity-preview-history";
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

const TARGET_PATH = "z-refresh-target.txt";
const PREFIX_PATH = "a-refresh-prefix.txt";
const UNRELATED_PATH = "m-refresh-unrelated.txt";
const INITIAL_MARKER = "initial-target-line-70";
const UPDATED_MARKER = "updated-target-line-77";

function targetContent(targetMarker: string, shifted = false) {
  const lines = Array.from({ length: 140 }, (_, index) => `stable-target-line-${index + 1}`);
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
      { timeout: 30_000, message: `diff content should ${present ? "show" : "hide"} ${text}` },
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

test.describe("desktop Git diff refresh continuity", () => {
  test.describe.configure({ timeout: 120_000 });

  for (const renderer of ["pierre-diffs", "monaco"] as const) {
    test(`retains counts and reading state with ${renderer}`, async ({
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
      const initialHead = git.getCurrentSha();
      const restorePreviewHistory = seedContinuityPreviewHistory(git);
      git.createFile(PREFIX_PATH, prefixContent(16, "prefix-before"));
      git.createFile(TARGET_PATH, targetContent(INITIAL_MARKER));

      await setDiffRenderer(testPage, renderer);
      const bridge = await routeGitStatusRefresh(testPage, { dropPendingStatusEvents: false });
      const gate = createGitEnrichmentGate(backend.tmpDir);
      const title = `Desktop Diff Refresh Continuity ${renderer}`;

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
        await session.clickTab("Changes");
        const changes = session.changes;
        await expect(changes.getByTestId(`file-row-${TARGET_PATH}`)).toBeVisible({
          timeout: 30_000,
        });
        await expect(
          changes.getByTestId(`file-row-${TARGET_PATH}`).getByText("+140", { exact: true }),
        ).toBeVisible();
        await bridge.waitForResponse("fresh", priorFreshResponses);
        await openAllChangesDiff(changes, testPage);

        const diffRoot = testPage.getByTestId("review-diff-scroll");
        await expect(diffRoot.locator("[data-review-file-key]")).toHaveCount(14);
        const targetSection = diffRoot.locator(
          `[data-review-file-key="${encodeURIComponent(TARGET_PATH)}"]`,
        );
        await expect(targetSection).toBeVisible({ timeout: 30_000 });
        await scrollDiffIntoReadingPosition(testPage, renderer, TARGET_PATH);
        const anchorBefore = await visibleDiffAnchor(testPage, renderer, TARGET_PATH);
        expect(anchorBefore?.line).toBeGreaterThan(30);
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        await rememberDiffViewerNode(testPage, renderer, TARGET_PATH);
        await prCapture.screenshot(`git-diff-continuity-desktop-${renderer}-ready`, {
          caption: `A long ${renderer} diff is ready at a manually selected reading position`,
        });

        const firstRefresh = await holdRefreshDuringEdit(testPage, bridge, gate, TARGET_PATH, () =>
          git.createFile(UNRELATED_PATH, "unrelated changed file\n"),
        );
        const retainedStatus = targetSection.getByTestId("review-header-refresh-status");
        await expect(retainedStatus).toContainText("Diff is loading");
        await expect(
          changes.getByTestId(`file-row-${TARGET_PATH}`).getByText("+140", { exact: true }),
        ).toBeVisible();
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        expectSameAnchor(anchorBefore, await visibleDiffAnchor(testPage, renderer, TARGET_PATH));
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        await prCapture.screenshot(`git-diff-continuity-desktop-${renderer}-pending`, {
          caption:
            "A real pending Git notification keeps the prior counts and readable diff in place",
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
            git.createFile(PREFIX_PATH, prefixContent(30, "prefix-expanded"));
            git.createFile(TARGET_PATH, targetContent(UPDATED_MARKER, true));
          },
        );
        await expect(retainedStatus).toContainText("Diff is loading");
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, true);
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, false);
        await expect(
          changes.getByTestId(`file-row-${TARGET_PATH}`).getByText("+140", { exact: true }),
        ).toBeVisible();

        await releaseRefreshEnrichment(bridge, gate, secondRefresh.priorReadyNotifications);
        await expect(retainedStatus).toHaveCount(0);
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, true);
        await expectDiffText(testPage, renderer, TARGET_PATH, INITIAL_MARKER, false);
        await expect(
          changes.getByTestId(`file-row-${TARGET_PATH}`).getByText("+147", { exact: true }),
        ).toBeVisible();
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        const changedAnchor = await visibleDiffAnchor(testPage, renderer, TARGET_PATH);
        expectSameAnchor(anchorBefore, changedAnchor);
        expect(changedAnchor?.line).not.toBe(anchorBefore?.line);
        await prCapture.screenshot(`git-diff-continuity-desktop-${renderer}-changed`, {
          caption:
            "Changed content above and inside the diff restores the same surviving line anchor",
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
        await expect(
          changes.getByTestId(`file-row-${TARGET_PATH}`).getByText("+147", { exact: true }),
        ).toBeVisible();
        await expectDiffText(testPage, renderer, TARGET_PATH, UPDATED_MARKER, true);
        expect(await isSameDiffViewerNode(testPage, renderer, TARGET_PATH)).toBe(true);
        bridge.allowResponses();

        git.exec(`rm ${TARGET_PATH}`);
        await triggerForegroundRefresh(testPage);
        await expect(targetSection).toHaveCount(0, { timeout: 30_000 });
        await expect(changes.getByTestId(`file-row-${TARGET_PATH}`)).toHaveCount(0, {
          timeout: 30_000,
        });
        const scrollBounds = await diffRoot.evaluate((element) => ({
          top: (element as HTMLElement).scrollTop,
          max: (element as HTMLElement).scrollHeight - (element as HTMLElement).clientHeight,
        }));
        expect(scrollBounds.top).toBeGreaterThanOrEqual(0);
        expect(scrollBounds.top).toBeLessThanOrEqual(scrollBounds.max);
        await prCapture.screenshot(`git-diff-continuity-desktop-${renderer}-removed`, {
          caption: "A complete membership update removes the file and retires its old diff",
        });
      } finally {
        gate.dispose();
        try {
          restorePreviewHistory();
        } finally {
          git.deleteFile(PREFIX_PATH);
          git.deleteFile(TARGET_PATH);
          git.deleteFile(UNRELATED_PATH);
        }
      }
      expect(git.getCurrentSha()).toBe(initialHead);
      expect(
        git.exec(`git status --porcelain -- ${PREFIX_PATH} ${TARGET_PATH} ${UNRELATED_PATH}`),
      ).toBe("");
    });
  }
});
