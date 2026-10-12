import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  ensureManagedConversation,
  installAndGrantManagedConversationAccess,
  seedManagedAutomation,
} from "./managed-automation-helpers";

test.describe("managed conversation automation on phone", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("phone destination editor", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    await installAndGrantManagedConversationAccess(testPage, apiClient, seedData.workspaceId);
    const destination = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "phone-e2e-managed",
      seedData.agentProfileId,
    );
    const automation = await seedManagedAutomation(
      apiClient,
      seedData.workspaceId,
      "Phone managed schedule",
      destination,
    );

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    const editor = testPage.getByTestId("automation-editor");
    await expect(editor).toBeVisible();
    await expect(
      testPage.getByRole("radio", { name: "Managed conversation", exact: false }),
    ).toBeChecked();

    const trigger = testPage.getByTestId("managed-conversation-destination-trigger");
    await expect(trigger).toContainText("phone-e2e-managed");
    await expect
      .poll(async () => (await trigger.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const drawer = testPage.getByTestId("managed-conversation-destination-drawer");
    await expect(drawer).toBeVisible();
    const option = drawer.getByText("Kandev E2E Fixture Plugin / phone-e2e-managed", {
      exact: true,
    });
    await expect(option).toBeVisible();
    await option.tap();
    await expect(drawer).toBeHidden();
    await expect(trigger).toContainText("phone-e2e-managed");
  });
  test("loads persisted retries as a dirty disabled draft and saves the reset", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installAndGrantManagedConversationAccess(testPage, apiClient, seedData.workspaceId);
    const destination = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "phone-legacy-retry",
      seedData.agentProfileId,
    );
    const enabledPolicy = {
      mode: "finite" as const,
      max_retries: "3",
      delay_seconds: "15",
      backoff: "exponential" as const,
      history_mode: "timeline" as const,
    };
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Phone legacy retry policy",
      taskMode: "managed_conversation",
      managedDestination: destination,
      retryPolicy: enabledPolicy,
      prompt: "Report blockers from the workspace.",
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "scheduled",
      config: { cron_expression: "0 9 * * 1-5", timezone: "UTC" },
      enabled: true,
    });

    const sentActions: string[] = [];
    testPage.on("websocket", (socket) => {
      if (!socket.url().endsWith("/ws")) return;
      socket.on("framesent", ({ payload }) => {
        if (typeof payload !== "string") return;
        try {
          const frame = JSON.parse(payload) as { action?: string };
          if (frame.action === "automation.update") sentActions.push(frame.action);
        } catch {
          // Ignore non-JSON gateway frames.
        }
      });
    });
    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await expect(testPage.getByTestId("automation-editor")).toBeVisible();
    await expect(testPage.locator("#automation-retry-finite")).toHaveCount(0);
    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /save changes/i });
    await expect(saveButton).toBeEnabled();
    expect(sentActions).toEqual([]);

    await saveButton.tap();
    await expect(saveButton).toHaveCount(0);
    await expect.poll(() => sentActions).toEqual(["automation.update"]);

    await testPage.reload();
    await expect(testPage.getByTestId("automation-editor")).toBeVisible();
    await expect(saveButton).toHaveCount(0);
    await testPage.getByRole("radio", { name: /Create a normal task/ }).tap();
    await expect(testPage.getByRole("radio", { name: "Do not retry" })).toBeChecked();
  });
});
