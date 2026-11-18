import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { createStandardProfile } from "./git-helper";
import { SessionPage } from "../pages/session-page";

export async function checkMarkdownOpening({
  testPage,
  apiClient,
  seedData,
  mobile,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  mobile: boolean;
}) {
  const profile = await createStandardProfile(apiClient, `markdown-default-${Date.now()}`);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Markdown default",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  let root = "";
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        root = environment?.repos?.[0]?.worktree_path ?? environment?.workspace_path ?? "";
        return environment?.status === "ready" && !!root && fs.existsSync(root);
      },
      { timeout: 30000 },
    )
    .toBe(true);
  fs.writeFileSync(path.join(root, "preview-on.md"), "# Preview On\n\nRead this file.");
  fs.writeFileSync(path.join(root, "preview-off.md"), "# Preview Off\n\nRead this file.");
  const original = await apiClient.rawRequest("GET", "/api/v1/user/settings");
  const originalValue = (await original.json()).settings.markdown_preview_by_default;
  try {
    for (const enabled of [true, false]) {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        markdown_preview_by_default: enabled,
      });
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      if (enabled) {
        await session.waitForLoad();
        await session.waitForChatIdle();
      }
      if (mobile) await testPage.getByRole("button", { name: "Files", exact: true }).tap();
      else await session.clickTab("Files");
      const file = await session.fileTree.waitForFileTreeNode(
        enabled ? "preview-on.md" : "preview-off.md",
      );
      if (mobile) await file.tap();
      else await file.click();
      const preview = testPage.getByTestId("markdown-preview");
      if (enabled) {
        await expect(preview).toBeVisible();
        const sourceToggle = preview.getByTestId("markdown-preview-toggle");
        if (mobile) await sourceToggle.tap();
        else await sourceToggle.click();
      }
      await expect(preview).not.toBeVisible();
      await expect(testPage.locator(".cm-editor:visible, .monaco-editor:visible")).toBeVisible();
      const previewToggle = testPage.getByTestId("markdown-preview-toggle");
      if (mobile) await previewToggle.tap();
      else await previewToggle.click();
      await expect(preview).toBeVisible();
    }
  } finally {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      markdown_preview_by_default: originalValue,
    });
  }
}
