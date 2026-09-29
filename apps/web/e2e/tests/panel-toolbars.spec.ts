import { expect, test } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";
import { GitHelper, makeGitEnv } from "../helpers/git-helper";
import { enableCanvasFeature, removeCanvas, seedTaskCanvas } from "./canvas/canvas-fixture";

type HeaderGeometry = {
  height: number;
  top: number;
  bottom: number;
  clientHeight: number;
  scrollHeight: number;
  flexWrap: string;
  nextTop: number | null;
};

async function createToolbarTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return session;
}

async function readVisibleHeaderGeometry(panel: Locator): Promise<HeaderGeometry[]> {
  return panel.locator("[data-panel-header]:visible").evaluateAll((headers) =>
    headers.map((header) => {
      const rect = header.getBoundingClientRect();
      const next = header.nextElementSibling;
      const nextRect = next?.getBoundingClientRect();
      const style = window.getComputedStyle(header);
      return {
        height: rect.height,
        top: rect.top,
        bottom: rect.bottom,
        clientHeight: header.clientHeight,
        scrollHeight: header.scrollHeight,
        flexWrap: style.flexWrap,
        nextTop: nextRect && nextRect.width > 0 && nextRect.height > 0 ? nextRect.top : null,
      };
    }),
  );
}

async function expectHeadersAtHeight(panel: Locator, expectedHeight: number, surface: string) {
  await expect
    .poll(() => readVisibleHeaderGeometry(panel), {
      timeout: 15_000,
      message: `Waiting for a visible shared header in ${surface}`,
    })
    .not.toEqual([]);

  const headers = await readVisibleHeaderGeometry(panel);
  expect(headers.length, `expected visible shared header in ${surface}`).toBeGreaterThan(0);
  for (const header of headers) {
    expect(
      Math.abs(header.height - expectedHeight),
      `${surface} header height`,
    ).toBeLessThanOrEqual(1);
    expect(header.scrollHeight).toBeLessThanOrEqual(header.clientHeight + 1);
    expect(header.flexWrap).toBe("nowrap");
    if (header.nextTop !== null) {
      expect(
        Math.abs(header.nextTop - header.bottom),
        `${surface} content edge`,
      ).toBeLessThanOrEqual(1);
    }
  }
}

async function expectNoDocumentOverflow(page: Page, surface: string) {
  await expect
    .poll(
      () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
      { timeout: 5_000, message: `${surface} has document horizontal overflow` },
    )
    .toBe(true);
}

async function expectHeaderControlsContained(panel: Locator, surface: string) {
  await expect
    .poll(
      () =>
        panel.locator("[data-panel-header]:visible").evaluateAll((headers) => {
          const failures: string[] = [];
          for (const header of headers) {
            const headerRect = header.getBoundingClientRect();
            const controls = header.querySelectorAll<HTMLElement>(
              "button, a, input, select, textarea, [role='button']",
            );
            for (const control of controls) {
              const style = window.getComputedStyle(control);
              if (style.display === "none" || style.visibility === "hidden") continue;
              const rect = control.getBoundingClientRect();
              if (rect.width === 0 || rect.height === 0) continue;
              if (
                rect.left < headerRect.left - 1 ||
                rect.right > headerRect.right + 1 ||
                rect.top < headerRect.top - 1 ||
                rect.bottom > headerRect.bottom + 1
              ) {
                failures.push(
                  control.getAttribute("aria-label") ?? control.textContent ?? "control",
                );
              }
            }
          }
          return failures;
        }),
      { timeout: 5_000, message: `${surface} has controls outside its header row` },
    )
    .toEqual([]);
}

async function constrainDockviewPanel(panel: import("@playwright/test").Locator, width: number) {
  await panel.evaluate((element, panelWidth) => {
    const target =
      element.closest<HTMLElement>(".dv-panel-view, .dv-panel") ??
      (element.hasAttribute("data-portal-panel") ? element.parentElement : element) ??
      element;
    target.style.flex = `0 0 ${panelWidth}px`;
    target.style.minWidth = `${panelWidth}px`;
    target.style.width = `${panelWidth}px`;
  }, width);
}

test.describe("shared panel toolbars", () => {
  test("keeps migrated panel families at one compact row on desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await createToolbarTask(testPage, apiClient, seedData, "Panel toolbar desktop");

    await session.clickTab("Files");
    await expect(session.files).toBeVisible();
    await expectHeadersAtHeight(session.files, 30, "Files");
    await expectNoDocumentOverflow(testPage, "Files");

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    await expectHeadersAtHeight(session.changes, 30, "Changes");
    await expectNoDocumentOverflow(testPage, "Changes");

    await session.addBrowserPanel();
    await expect(session.browserPanel).toBeVisible();
    await session.browserAddressInput.fill(
      "https://example.test/a-very-long-preview-path-that-must-remain-in-the-toolbar-input",
    );
    await expectHeadersAtHeight(session.browserPanel, 30, "Browser");
    await expectNoDocumentOverflow(testPage, "Browser");
  });

  test("keeps the fine-pointer compact geometry at the 768px boundary", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 768, height: 900 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(false);

    const session = await createToolbarTask(
      testPage,
      apiClient,
      seedData,
      "Panel toolbar boundary",
    );
    await session.clickTab("Files");
    await expect(session.files).toBeVisible();
    await expectHeadersAtHeight(session.files, 30, "768px Files");
    await expectNoDocumentOverflow(testPage, "768px Files");
  });

  test("folds narrow Changes and Browser actions into reachable menus", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const session = await createToolbarTask(testPage, apiClient, seedData, "Narrow panel actions");
    const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
    git.createFile("narrow-toolbar-actions.ts", "export const narrowToolbarActions = true;\n");

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    await expect(session.changesFileRow("narrow-toolbar-actions.ts")).toBeVisible({
      timeout: 15_000,
    });
    await constrainDockviewPanel(session.changes, 240);
    await expect(session.changes.getByTestId("panel-header-overflow").first()).toBeVisible();
    await session.changes.getByTestId("panel-header-overflow").first().click();
    await expect(testPage.getByRole("menuitem", { name: "Diff", exact: true })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Review", exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectHeadersAtHeight(session.changes, 30, "narrow Changes");

    await session.addBrowserPanel();
    await expect(session.browserPanel).toBeVisible();
    await constrainDockviewPanel(session.browserPanel, 240);
    await expect(session.browserPanel.getByTestId("panel-header-overflow")).toBeVisible();
    await expectHeadersAtHeight(session.browserPanel, 30, "narrow Browser");
    await expectNoDocumentOverflow(testPage, "narrow panel actions");
  });

  test("keeps the multiple-PR selector and review actions reachable in a narrow panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("reviewer");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multiple PR toolbar actions",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, checkout_branch: "main" }],
      },
    );
    const taskBranch = "main";
    await apiClient.mockGitHubAddPRs([
      {
        number: 401,
        title: "Toolbar review one",
        state: "open",
        head_branch: taskBranch,
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 402,
        title: "Toolbar review two",
        state: "open",
        head_branch: taskBranch,
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 401, [
      { filename: "toolbar-one.ts", status: "added", additions: 1, deletions: 0 },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "testrepo", 402, [
      { filename: "toolbar-two.ts", status: "added", additions: 1, deletions: 0 },
    ]);
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 401,
      pr_url: "https://github.com/testorg/testrepo/pull/401",
      pr_title: "Toolbar review one",
      head_branch: taskBranch,
      base_branch: "main",
      author_login: "reviewer",
    });
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 402,
      pr_url: "https://github.com/testorg/testrepo/pull/402",
      pr_title: "Toolbar review two",
      head_branch: taskBranch,
      base_branch: "main",
      author_login: "reviewer",
    });

    await expect.poll(() => apiClient.listTaskPRs(task.id), { timeout: 10_000 }).toHaveLength(2);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    const changesOverflow = session.changes.getByTestId("panel-header-overflow").first();
    await expect(changesOverflow).toBeVisible({ timeout: 20_000 });
    await changesOverflow.click();
    await testPage.getByRole("menuitem", { name: "Diff", exact: true }).click();

    const selector = testPage.getByTestId("changes-review-pr-selector-trigger");
    await expect(selector).toBeVisible({ timeout: 20_000 });
    const diffHeader = selector.locator("xpath=ancestor::*[@data-panel-header]");
    await constrainDockviewPanel(diffHeader, 240);
    await expect(selector).toBeVisible();
    await expectHeaderControlsContained(
      diffHeader.locator("xpath=.."),
      "narrow multiple-PR Changes",
    );

    const overflow = diffHeader.getByTestId("panel-header-overflow");
    await expect(overflow).toBeVisible();
    await overflow.click();
    await expect(
      testPage.getByRole("menuitem", { name: "Expand review", exact: true }),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectNoDocumentOverflow(testPage, "narrow multiple-PR Changes");
  });

  test("keeps narrow embedded canvas actions reachable beside the workbench", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await enableCanvasFeature(backend, apiClient, seedData.workspaceId);
    let canvasId: string | undefined;
    try {
      const seeded = await seedTaskCanvas(testPage, apiClient, seedData);
      canvasId = seeded.canvas.id;
      const canvasPanel = testPage.getByTestId("canvas-host-panel");

      await expect(canvasPanel).toBeVisible({ timeout: 20_000 });
      await expect(testPage.getByTestId("canvas-host-state")).toHaveText("Ready", {
        timeout: 20_000,
      });
      await constrainDockviewPanel(canvasPanel, 240);

      const overflow = canvasPanel.getByTestId("panel-header-overflow");
      await expect(overflow).toBeVisible();
      await overflow.click();
      await expect(
        testPage.getByRole("menuitem", { name: "Releases and permissions", exact: true }),
      ).toBeVisible();
      await expect(
        testPage.getByRole("menuitem", { name: "Share canvas", exact: true }),
      ).toBeVisible();
      await expect(
        testPage.getByRole("menuitem", { name: "Promote canvas", exact: true }),
      ).toBeVisible();
      await testPage.keyboard.press("Escape");

      await expectHeaderControlsContained(canvasPanel, "narrow embedded canvas");
      await expectNoDocumentOverflow(testPage, "narrow embedded canvas");
    } finally {
      if (canvasId) await removeCanvas(apiClient, canvasId);
      await releaseFeature();
    }
  });

  test("keeps a dirty editor save action and secondary actions reachable when narrow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await createToolbarTask(testPage, apiClient, seedData, "Narrow editor actions");

    await session.clickTab("Files");
    await expect(session.files).toBeVisible();
    const baseFile = await session.fileTree.waitForFileTreeNode("walkthrough_base.txt");
    await baseFile.click();

    const editor = testPage.locator(".monaco-editor:visible").first();
    await expect(editor).toBeVisible({ timeout: 20_000 });
    await editor.click();
    await expect(editor.locator(".native-edit-context")).toBeFocused({ timeout: 5_000 });
    await testPage.keyboard.press("End");
    await testPage.keyboard.insertText("\n// toolbar coverage");

    const editorPanel = testPage.locator('[data-portal-panel="preview:file-editor"]');
    await expect(editorPanel.locator("[data-panel-header]:visible")).toHaveCount(1, {
      timeout: 10_000,
    });
    await constrainDockviewPanel(editorPanel, 240);
    const editorHeader = editorPanel.locator("[data-panel-header]:visible").first();
    await expect(editorHeader.getByTestId("panel-header-overflow")).toBeVisible();
    await expect(editorHeader.getByRole("button", { name: /Save/ })).toBeEnabled({
      timeout: 10_000,
    });
    await expectHeaderControlsContained(editorPanel, "narrow dirty editor");

    await editorHeader.getByTestId("panel-header-overflow").click();
    await expect(testPage.getByRole("menuitem", { name: /word wrap/i })).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectNoDocumentOverflow(testPage, "narrow dirty editor");
  });

  test("uses touch geometry and contains controls at fine-pointer phone widths", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(false);
    await createToolbarTask(testPage, apiClient, seedData, "Fine pointer phone toolbar");
    const mobileFiles = testPage.getByTestId("files-panel");
    const mobileChanges = testPage.getByTestId("mobile-changes-panel");

    for (const width of [390, 767]) {
      await testPage.setViewportSize({ width, height: 844 });
      await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
      await testPage.getByRole("button", { name: "Files", exact: true }).click();
      await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
      await expectHeadersAtHeight(mobileFiles, 48, `${width}px fine-pointer Files`);
      await expectHeaderControlsContained(mobileFiles, `${width}px fine-pointer Files`);
      await expect(testPage.getByRole("button", { name: "Search files", exact: true })).toHaveCSS(
        "height",
        "44px",
      );
      await expectNoDocumentOverflow(testPage, `${width}px fine-pointer Files`);

      await testPage.getByRole("button", { name: /Changes/ }).click();
      await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
      await expectHeadersAtHeight(mobileChanges, 48, `${width}px fine-pointer Changes`);
      await expectHeaderControlsContained(mobileChanges, `${width}px fine-pointer Changes`);
      await expectNoDocumentOverflow(testPage, `${width}px fine-pointer Changes`);
    }
  });
});
