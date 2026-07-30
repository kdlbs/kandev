import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import { rewriteBackendHostOS } from "../../helpers/boot-payload";
import { SessionPage } from "../../pages/session-page";

async function createHostedEditor(apiClient: ApiClient): Promise<void> {
  const response = await apiClient.rawRequest("POST", "/api/v1/editors", {
    name: "Hosted Preview",
    kind: "custom_hosted_url",
    config: { url: "https://example.com/editor" },
    enabled: true,
  });
  expect(response.ok).toBe(true);
}

async function seedTaskAndOpenEditorMenu(
  page: Page,
  apiClient: ApiClient,
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
    agentProfileId: string;
  },
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Host platform editor availability",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await page.getByTestId("editors-menu-list").click();
}

test.describe("host platform controls embedded VS Code availability", () => {
  test("hides embedded VS Code for a Windows backend and keeps custom editors", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await createHostedEditor(apiClient);
    await rewriteBackendHostOS(testPage, "windows");

    await seedTaskAndOpenEditorMenu(testPage, apiClient, seedData);

    const menu = testPage.getByRole("menu");
    await expect(menu.getByRole("menuitem", { name: "Hosted Preview" })).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: "VS Code (Embedded)" })).toHaveCount(0);
    await prCapture.screenshot("windows-host-editor-menu-desktop", {
      caption: "Windows-hosted Kandev editor menu with embedded VS Code unavailable",
    });
  });

  test("ignores a Windows browser platform when the backend host is Linux", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await createHostedEditor(apiClient);
    await testPage.addInitScript(() => {
      Object.defineProperty(Navigator.prototype, "platform", {
        configurable: true,
        get: () => "Win32",
      });
    });

    await seedTaskAndOpenEditorMenu(testPage, apiClient, seedData);

    await expect(
      testPage.getByRole("menu").getByRole("menuitem", { name: "VS Code (Embedded)" }),
    ).toBeVisible();
  });
});
