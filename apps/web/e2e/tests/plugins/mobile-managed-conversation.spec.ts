import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { linkToTask } from "@/lib/links";
import {
  ensureManagedConversation,
  installAndGrantManagedChatAccess,
  openManagedChatPage,
} from "./managed-automation-helpers";

test.describe("managed conversation chat on phone", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("phone tabs, drawer, keyboard and long content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installAndGrantManagedChatAccess(testPage, apiClient, seedData.workspaceId);
    const main = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "phone-chat-main",
      seedData.agentProfileId,
    );
    await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "phone-chat-reviewer",
      seedData.agentProfileId,
    );
    if (!main.session_id) throw new Error("Managed chat fixture did not return a session ID");
    await apiClient.seedAgentMessages(
      main.session_id,
      35,
      "Long conversation entry with a narrow mobile layout",
    );

    await openManagedChatPage(testPage, {
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      selectedInstanceKey: main.instance_key,
      instances: [
        { key: main.instance_key, label: "Delivery lead" },
        { key: "phone-chat-reviewer", label: "Reviewer" },
      ],
    });

    await expect(testPage.getByTestId("workspace-agent-chat")).toBeVisible();
    const chatTab = testPage.getByRole("tab", { name: "Chat" });
    const tasksTab = testPage.getByRole("tab", { name: "Tasks" });
    const outcomesTab = testPage.getByRole("tab", { name: "Outcomes" });
    await expect(chatTab).toBeVisible();
    await expect(tasksTab).toBeVisible();
    await expect(outcomesTab).toBeVisible();
    for (const control of [
      testPage.getByRole("button", { name: "Pause" }),
      testPage.getByRole("button", { name: "Send" }),
    ]) {
      await expect
        .poll(async () => (await control.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual(44);
    }
    await expect(testPage.getByTestId("managed-chat-transcript")).toContainText(
      "Long conversation entry with a narrow mobile layout 35",
    );

    const transcript = testPage.getByTestId("managed-chat-transcript");
    const transcriptGeometry = await transcript.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(transcriptGeometry.scrollHeight).toBeGreaterThan(transcriptGeometry.clientHeight);
    expect(transcriptGeometry.scrollWidth).toBeLessThanOrEqual(transcriptGeometry.clientWidth + 1);

    const picker = testPage.getByTestId("managed-chat-instance-picker");
    await expect
      .poll(async () => (await picker.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await picker.tap();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    await drawer.getByRole("button", { name: "Reviewer" }).tap();
    await expect(picker).toContainText("Reviewer");

    await tasksTab.focus();
    await testPage.keyboard.press("Enter");
    await expect(testPage.getByTestId("workspace-task-status")).toBeVisible();
    await expect(testPage.getByTestId("workspace-task-usage")).toBeVisible();
    await outcomesTab.focus();
    await testPage.keyboard.press("Enter");
    await expect(testPage.getByText("No outcomes to show yet.")).toBeVisible();

    await chatTab.focus();
    await testPage.keyboard.press("Enter");
    const composer = testPage.getByLabel("Message");
    await composer.tap();
    await composer.fill("Phone keyboard follow-up");
    await testPage.keyboard.press("Tab");
    await testPage.keyboard.press("Enter");
    await expect(composer).toHaveValue("");
    await expect(testPage.getByTestId("managed-chat-controls")).toContainText(
      "Phone keyboard follow-up",
    );

    const pageGeometry = await testPage.locator("body").evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(pageGeometry.scrollWidth).toBeLessThanOrEqual(pageGeometry.clientWidth + 1);

    if (!main.task_id || !main.session_id) {
      throw new Error("Managed chat fixture did not return task and session IDs");
    }
    await uninstallFixturePlugin(apiClient);
    await testPage.goto(linkToTask(main.task_id, { sessionId: main.session_id }));
    const retained = testPage.getByTestId("managed-retained-transcript");
    await expect(retained).toBeVisible();
    await expect(retained).toContainText("Long conversation entry with a narrow mobile layout 35");
    await expect(testPage.getByLabel("Message")).toHaveCount(0);
    const retainedGeometry = await testPage.locator("body").evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(retainedGeometry.scrollWidth).toBeLessThanOrEqual(retainedGeometry.clientWidth + 1);
  });
});
