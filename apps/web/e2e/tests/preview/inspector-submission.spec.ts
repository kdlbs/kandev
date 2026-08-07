import http from "node:http";
import type { AddressInfo } from "node:net";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { openTaskSession, createStandardProfile } from "../../helpers/git-helper";

const MOCK_HTML = `<!DOCTYPE html>
<html>
<head><title>Mock Dev Server</title></head>
<body>
  <button id="submit-btn" role="button" aria-label="Submit form">Submit</button>
</body>
</html>`;

async function startMockDevServer(): Promise<{ port: number; close: () => Promise<void> }> {
  const server = http.createServer((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(MOCK_HTML);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  return {
    port,
    close: () =>
      new Promise<void>((resolve, reject) =>
        server.close((err) => (err ? reject(err) : resolve())),
      ),
  };
}

async function openPreview(
  testPage: import("@playwright/test").Page,
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string },
  title: string,
  port: number,
) {
  await apiClient.updateRepository(seedData.repositoryId, { dev_script: "echo preview" });
  const profile = await createStandardProfile(apiClient, `inspector-${title.toLowerCase()}`);
  await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description: "preview inspector submission",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  const session = await openTaskSession(testPage, title);
  await session.waitForChatIdle({ timeout: 30_000 });
  const openBrowserPreview = testPage.getByRole("button", { name: /open browser preview/i });
  await expect(openBrowserPreview).toBeVisible();
  await openBrowserPreview.click();

  const urlInput = testPage.locator('[placeholder*="localhost"], [placeholder*="3000"]').first();
  await expect(urlInput).toBeVisible();
  await urlInput.fill(`http://localhost:${port}`);
  await urlInput.press("Enter");

  const iframe = testPage.locator('iframe[title*="preview" i]');
  await expect(iframe).toBeVisible({ timeout: 15_000 });
  const preview = testPage.frameLocator('iframe[title*="preview" i]');
  await expect(preview.locator("#submit-btn")).toBeVisible();

  await testPage.getByTestId("preview-inspect-button").click();
  await expect(preview.locator("#submit-btn")).toBeVisible();
  return preview;
}

test.describe("Browser inspect annotation submission", () => {
  test("Save submits a pin annotation", async ({ testPage, apiClient, seedData }) => {
    const devServer = await startMockDevServer();
    try {
      const preview = await openPreview(
        testPage,
        apiClient,
        seedData,
        "Inspector Save",
        devServer.port,
      );

      await preview.locator("#submit-btn").click();
      const comment = preview.locator("textarea");
      await expect(comment).toBeVisible();
      await comment.fill("make this primary color");
      await preview.getByRole("button", { name: "Save" }).click();

      await expect(preview.locator("textarea")).toHaveCount(0);
      await expect(preview.locator('[data-kandev-marker="1"]')).toBeVisible();
      await expect(testPage.getByTestId("preview-annotation-item")).toContainText(
        "make this primary color",
      );
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { dev_script: "" });
      await devServer.close();
    }
  });

  test("Enter submits a pin annotation", async ({ testPage, apiClient, seedData }) => {
    const devServer = await startMockDevServer();
    try {
      const preview = await openPreview(
        testPage,
        apiClient,
        seedData,
        "Inspector Enter",
        devServer.port,
      );

      await preview.locator("#submit-btn").click();
      const comment = preview.locator("textarea");
      await expect(comment).toBeVisible();
      await comment.fill("increase font size");
      await comment.press("Enter");

      await expect(preview.locator("textarea")).toHaveCount(0);
      await expect(preview.locator('[data-kandev-marker="1"]')).toBeVisible();
      await expect(testPage.getByTestId("preview-annotation-item")).toContainText(
        "increase font size",
      );
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { dev_script: "" });
      await devServer.close();
    }
  });
});
