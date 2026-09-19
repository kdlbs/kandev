import { expect, test } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

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

async function readVisibleHeaderGeometry(page: Page): Promise<HeaderGeometry[]> {
  return page.locator("[data-panel-header]:visible").evaluateAll((headers) =>
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

async function expectHeadersAtHeight(page: Page, expectedHeight: number, surface: string) {
  await expect
    .poll(() => readVisibleHeaderGeometry(page), {
      timeout: 15_000,
      message: `Waiting for a visible shared header in ${surface}`,
    })
    .not.toEqual([]);

  const headers = await readVisibleHeaderGeometry(page);
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

async function expectHeaderControlsContained(page: Page, surface: string) {
  const violations = await page.locator("[data-panel-header]:visible").evaluateAll((headers) => {
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
            `${control.getAttribute("aria-label") ?? control.textContent ?? "control"}`,
          );
        }
      }
    }
    return failures;
  });
  expect(violations, `${surface} has controls outside its header row`).toEqual([]);
}

async function constrainDockviewPanel(panel: import("@playwright/test").Locator, width: number) {
  await panel.evaluate((element, panelWidth) => {
    const target = element.closest<HTMLElement>(".dv-panel-view, .dv-panel") ?? element;
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
    await expectHeadersAtHeight(testPage, 30, "Files");
    await expectNoDocumentOverflow(testPage, "Files");

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    await expectHeadersAtHeight(testPage, 30, "Changes");
    await expectNoDocumentOverflow(testPage, "Changes");

    await session.addBrowserPanel();
    await expect(session.browserPanel).toBeVisible();
    await session.browserAddressInput.fill(
      "https://example.test/a-very-long-preview-path-that-must-remain-in-the-toolbar-input",
    );
    await expectHeadersAtHeight(testPage, 30, "Browser");
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
    await expectHeadersAtHeight(testPage, 30, "768px Files");
    await expectNoDocumentOverflow(testPage, "768px Files");
  });

  test("folds narrow Changes and Browser actions into reachable menus", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await createToolbarTask(testPage, apiClient, seedData, "Narrow panel actions");

    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible();
    await constrainDockviewPanel(session.changes, 240);
    await expect(session.changes.getByTestId("panel-header-overflow").first()).toBeVisible();
    await session.changes.getByTestId("panel-header-overflow").first().click();
    await expect(testPage.getByRole("menuitem", { name: "Diff", exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectHeadersAtHeight(testPage, 30, "narrow Changes");

    await session.addBrowserPanel();
    await expect(session.browserPanel).toBeVisible();
    await constrainDockviewPanel(session.browserPanel, 240);
    await expect(session.browserPanel.getByTestId("panel-header-overflow")).toBeVisible();
    await expectHeadersAtHeight(testPage, 30, "narrow Browser");
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
    await expectHeaderControlsContained(testPage, "narrow multiple-PR Changes");

    const overflow = diffHeader.getByTestId("panel-header-overflow");
    await expect(overflow).toBeVisible();
    await overflow.click();
    await expect(
      testPage.getByRole("menuitem", { name: "Expand review", exact: true }),
    ).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectNoDocumentOverflow(testPage, "narrow multiple-PR Changes");
  });

  test("uses touch geometry and contains controls at fine-pointer phone widths", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(false);
    await createToolbarTask(testPage, apiClient, seedData, "Fine pointer phone toolbar");

    for (const width of [390, 767]) {
      await testPage.setViewportSize({ width, height: 844 });
      await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
      await testPage.getByRole("button", { name: "Files", exact: true }).click();
      await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
      await expectHeadersAtHeight(testPage, 48, `${width}px fine-pointer Files`);
      await expectHeaderControlsContained(testPage, `${width}px fine-pointer Files`);
      await expect(testPage.getByRole("button", { name: "Search files", exact: true })).toHaveCSS(
        "height",
        "44px",
      );
      await expectNoDocumentOverflow(testPage, `${width}px fine-pointer Files`);

      await testPage.getByRole("button", { name: /Changes/ }).click();
      await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
      await expectHeadersAtHeight(testPage, 48, `${width}px fine-pointer Changes`);
      await expectHeaderControlsContained(testPage, `${width}px fine-pointer Changes`);
      await expectNoDocumentOverflow(testPage, `${width}px fine-pointer Changes`);
    }
  });
});
