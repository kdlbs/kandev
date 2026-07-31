import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { routeSessionEmbeddedVscodeCapability } from "../../helpers/session-capabilities";
import type { Page } from "@playwright/test";
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
    "Executor editor availability",
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

test.describe("executor capability controls embedded VS Code availability", () => {
  test("hides embedded VS Code for an unsupported session and keeps custom editors", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await routeSessionEmbeddedVscodeCapability(testPage, false);
    await createHostedEditor(apiClient);

    await seedTaskAndOpenEditorMenu(testPage, apiClient, seedData);

    const menu = testPage.getByRole("menu");
    await expect(menu.getByRole("menuitem", { name: "Hosted Preview" })).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: "VS Code (Embedded)" })).toHaveCount(0);
    await prCapture.screenshot("unsupported-executor-editor-menu-desktop", {
      caption: "Unsupported session hides embedded VS Code while custom editors remain available",
    });
  });

  test("shows embedded VS Code for a supported session despite a Windows browser", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await routeSessionEmbeddedVscodeCapability(testPage, true);
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
