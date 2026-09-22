import fs from "node:fs";
import path from "node:path";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

function messageScript(content: string): string {
  const escaped = content.replaceAll("\\", "\\\\").replaceAll('"', '\\"').replaceAll("\n", "\\n");
  return `e2e:message("${escaped}")`;
}

async function openMarkdownPreview(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  fileName: string,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Markdown Math Preview",
    seedData.agentProfileId,
    {
      description: messageScript("Preview formula file"),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  await expect(session.files).toBeVisible({ timeout: 5_000 });
  const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${fileName}"]`);
  await expect(fileNode).toBeVisible({ timeout: 10_000 });
  await fileNode.tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible({ timeout: 10_000 });
  const previewToggle = viewer.getByTestId("markdown-preview-toggle");
  await expect(previewToggle).toBeVisible({ timeout: 10_000 });
  await previewToggle.tap();
  await expect(viewer.getByTestId("markdown-preview")).toBeVisible({ timeout: 10_000 });
}

test.describe("mobile: Markdown math", () => {
  test("keeps wide display formulas in a local scroll region", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const wideFormula = [
      "\\frac{a_1 + b_1 + c_1 + d_1}{e_1 + f_1 + g_1}",
      " + \\frac{a_2 + b_2 + c_2 + d_2}{e_2 + f_2 + g_2}",
      " + \\frac{a_3 + b_3 + c_3 + d_3}{e_3 + f_3 + g_3}",
      " + \\frac{a_4 + b_4 + c_4 + d_4}{e_4 + f_4 + g_4}",
      " + \\frac{a_5 + b_5 + c_5 + d_5}{e_5 + f_5 + g_5}",
    ].join("");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Markdown Math",
      seedData.agentProfileId,
      {
        description: messageScript(["Inline: $E = mc^2$", "", "$$", wideFormula, "$$"].join("\n")),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    const display = chat.locator(".katex-display");
    await expect(display).toBeVisible({ timeout: 30_000 });
    await expect(chat.locator(".katex").first()).toBeVisible();

    const scrollMetrics = await display.evaluate((element) => {
      const maxScrollLeft = element.scrollWidth - element.clientWidth;
      element.scrollLeft = element.scrollWidth;
      return {
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        maxScrollLeft,
        scrollLeft: element.scrollLeft,
      };
    });
    expect(scrollMetrics.scrollWidth).toBeGreaterThan(scrollMetrics.clientWidth + 1);
    expect(scrollMetrics.maxScrollLeft).toBeGreaterThan(0);
    expect(scrollMetrics.scrollLeft).toBeGreaterThan(0);
    expect(await chat.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
      true,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });

  test("keeps wide display formulas in a file preview scroll region", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const fileName = "mobile-math-preview.md";
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    fs.writeFileSync(
      path.join(repoDir, fileName),
      [
        "# Formula preview",
        "",
        "$$",
        "\\frac{a_1 + b_1 + c_1 + d_1 + e_1 + f_1}{g_1 + h_1 + i_1 + j_1}",
        " + \\frac{a_2 + b_2 + c_2 + d_2 + e_2 + f_2}{g_2 + h_2 + i_2 + j_2}",
        " + \\frac{a_3 + b_3 + c_3 + d_3 + e_3 + f_3}{g_3 + h_3 + i_3 + j_3}",
        "$$",
      ].join("\n"),
    );

    await openMarkdownPreview(testPage, apiClient, seedData, fileName);

    const preview = testPage.getByTestId("markdown-preview");
    const display = preview.locator(".katex-display");
    await expect(display).toBeVisible();
    const scrollMetrics = await display.evaluate((element) => {
      const maxScrollLeft = element.scrollWidth - element.clientWidth;
      element.scrollLeft = element.scrollWidth;
      return {
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        maxScrollLeft,
        scrollLeft: element.scrollLeft,
      };
    });
    expect(scrollMetrics.scrollWidth).toBeGreaterThan(scrollMetrics.clientWidth + 1);
    expect(scrollMetrics.maxScrollLeft).toBeGreaterThan(0);
    expect(scrollMetrics.scrollLeft).toBeGreaterThan(0);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });
});
