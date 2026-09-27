import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { linkToTask } from "@/lib/links";
import {
  ensureManagedConversation,
  installAndGrantManagedChatAccess,
  openManagedChatPage,
} from "./managed-automation-helpers";

test.describe("managed conversation chat", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("conversation and lifecycle states", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await installAndGrantManagedChatAccess(testPage, apiClient, seedData.workspaceId);
    const main = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "desktop-chat-main",
      seedData.agentProfileId,
    );
    await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "desktop-chat-reviewer",
      seedData.agentProfileId,
    );

    let statusMode: "ready" | "offline" | "revoked" | "unsupported" = "ready";
    await testPage.route(
      "**/api/plugins/kandev-plugin-e2e/actions/managed-conversation-status",
      async (route) => {
        if (statusMode === "offline") {
          await route.abort("failed");
          return;
        }
        const response = await route.fetch();
        if (statusMode === "ready") {
          await route.fulfill({ response });
          return;
        }
        const body = (await response.json()) as Record<string, unknown>;
        body.managed_conversation_supported = false;
        body.managed_conversation_reason =
          statusMode === "revoked"
            ? "capability_not_approved"
            : "managed_conversation_service_unavailable";
        await route.fulfill({ response, json: body });
      },
    );

    await openManagedChatPage(testPage, {
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      selectedInstanceKey: main.instance_key,
      instances: [
        { key: main.instance_key, label: "Delivery lead" },
        { key: "desktop-chat-reviewer", label: "Reviewer" },
      ],
    });

    const chat = testPage.getByTestId("workspace-agent-chat");
    await expect(chat).toBeVisible();
    await expect(testPage.getByTestId("managed-chat-transcript")).toContainText("No messages yet");
    await expect(testPage.getByTestId("workspace-task-status")).toBeVisible();
    await expect(testPage.getByTestId("workspace-task-usage")).toBeVisible();
    for (const control of [
      testPage.getByTestId("managed-chat-instance-picker"),
      testPage.getByRole("button", { name: "Pause" }),
      testPage.getByRole("button", { name: "Send" }),
      testPage.getByTestId("managed-chat-composer").locator("textarea"),
    ]) {
      await expect.poll(async () => (await control.boundingBox())?.height ?? 0).toBe(28);
    }

    await testPage.getByTestId("managed-chat-instance-picker").click();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    await drawer.getByRole("button", { name: "Reviewer" }).click();
    await expect(testPage.getByTestId("managed-chat-instance-picker")).toContainText("Reviewer");
    await testPage.getByTestId("managed-chat-instance-picker").click();
    await testPage.getByRole("dialog").getByRole("button", { name: "Delivery lead" }).click();

    const actionAttempts: Array<Record<string, unknown>> = [];
    let enqueueAttempts = 0;
    await testPage.route(
      "**/api/plugins/kandev-plugin-e2e/actions/managed-conversation-enqueue",
      async (route) => {
        const request = route.request().postDataJSON() as Record<string, unknown>;
        const body = (request.body || request) as Record<string, unknown>;
        actionAttempts.push(body);
        enqueueAttempts += 1;
        if (enqueueAttempts === 1) {
          await route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({ error: "uncertain" }),
          });
          return;
        }
        await route.continue();
      },
    );
    const composer = testPage.getByLabel("Message");
    await composer.fill("Please report the next review blocker.");
    await testPage.getByRole("button", { name: "Send" }).click();
    await expect(testPage.getByRole("button", { name: "Retry" })).toBeVisible();
    await testPage.getByRole("button", { name: "Retry" }).click();
    await expect.poll(() => actionAttempts.length).toBe(2);
    for (const key of ["request_id", "idempotency_key", "occurrence_key", "payload"]) {
      expect(actionAttempts[1]?.[key]).toBe(actionAttempts[0]?.[key]);
    }
    await expect(testPage.getByTestId("managed-chat-controls")).toContainText("Input 1:");

    await testPage.getByRole("button", { name: "Pause" }).click();
    await expect(testPage.getByRole("button", { name: "Resume" })).toBeEnabled();
    await testPage.getByRole("button", { name: "Resume" }).click();
    await expect(testPage.getByRole("button", { name: "Pause" })).toBeEnabled();

    statusMode = "offline";
    await expect(chat).toContainText("Connection is unavailable", { timeout: 10_000 });
    statusMode = "revoked";
    await expect(chat).toContainText("Workspace access was revoked", { timeout: 10_000 });
    statusMode = "unsupported";
    await expect(chat).toContainText("does not support this managed conversation", {
      timeout: 10_000,
    });
    const settingsButton = testPage.getByRole("button", { name: "Open plugin settings" });
    await expect(settingsButton).toBeVisible();
    await expect.poll(async () => (await settingsButton.boundingBox())?.height ?? 0).toBe(28);

    if (!main.task_id || !main.session_id) {
      throw new Error("Managed chat fixture did not return task and session IDs");
    }
    await uninstallFixturePlugin(apiClient);
    await testPage.goto(linkToTask(main.task_id, { sessionId: main.session_id }));
    const retained = testPage.getByTestId("managed-retained-transcript");
    await expect(retained).toBeVisible();
    await expect(retained).toContainText("read-only");
    await expect(testPage.getByLabel("Message")).toHaveCount(0);
  });
});
