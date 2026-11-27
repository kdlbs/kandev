import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  COORDINATOR_PLUGIN_ID,
  invokeCoordinatorAction,
  installAndGrantCoordinator,
  uninstallCoordinator,
  verifyCoordinatorRetryStaysWithInstance,
} from "./reference-coordinator-helpers";

type CoordinatorInstance = {
  key: string;
  name: string;
  agent_profile_id: string;
  instructions: string;
};

test.describe("packaged reference coordinator", () => {
  let alternateProfileId = "";

  test.afterEach(async ({ apiClient }) => {
    await uninstallCoordinator(apiClient);
    if (alternateProfileId) {
      await apiClient.deleteAgentProfile(alternateProfileId, true).catch(() => undefined);
      alternateProfileId = "";
    }
  });

  test("packaged instance creation and delegation", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);

    const { agents } = await apiClient.listAgents();
    const owner = agents.find((agent) =>
      agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
    );
    if (!owner)
      throw new Error(`Seed agent profile ${seedData.agentProfileId} has no owning agent`);
    const baseProfile = await apiClient.getAgentProfile(seedData.agentProfileId);
    const alternateProfile = await apiClient.createAgentProfile(
      owner.id,
      `Coordinator reviewer ${randomUUID().slice(0, 8)}`,
      {
        model: baseProfile.model,
        auto_approve: true,
      },
    );
    alternateProfileId = alternateProfile.id;

    await testPage.goto(`/plugins/${COORDINATOR_PLUGIN_ID}`);
    await expect(testPage.getByTestId("coordinator-empty")).toBeVisible();
    await testPage.getByTestId("coordinator-add-instance").click();
    await expect(testPage.getByTestId("coordinator-settings")).toBeVisible();
    await testPage.getByTestId("coordinator-field-name").fill("Delivery lead");
    await testPage.getByTestId("coordinator-field-role").selectOption("delivery-lead");
    await testPage
      .getByTestId("coordinator-field-agent-profile")
      .selectOption(seedData.agentProfileId);
    await testPage
      .getByTestId("coordinator-field-instructions")
      .fill("Track delivery risks and delegated work.");
    await testPage.getByTestId("coordinator-save").click();
    await expect(testPage.getByTestId("coordinator-page")).toBeVisible();

    await testPage.getByTestId("coordinator-add-instance").click();
    await expect(testPage.getByTestId("coordinator-settings")).toBeVisible();
    await testPage.getByTestId("coordinator-field-name").fill("Reviewer");
    await testPage.getByTestId("coordinator-field-role").selectOption("code-reviewer");
    await testPage.getByTestId("coordinator-field-agent-profile").selectOption(alternateProfileId);
    await testPage
      .getByTestId("coordinator-field-instructions")
      .fill("Review changes and report only verified findings.");
    await testPage.getByTestId("coordinator-save").click();
    await expect(testPage.getByTestId("coordinator-page")).toBeVisible();

    const list = await invokeCoordinatorAction<{ instances: CoordinatorInstance[] }>(
      apiClient,
      seedData.workspaceId,
      "instance.list",
    );
    expect(list.instances).toHaveLength(2);
    expect(new Set(list.instances.map((instance) => instance.agent_profile_id)).size).toBe(2);
    expect(new Set(list.instances.map((instance) => instance.instructions)).size).toBe(2);

    const deliveryLead = list.instances.find((instance) => instance.name === "Delivery lead");
    if (!deliveryLead) throw new Error("Delivery lead coordinator instance was not returned");
    await verifyCoordinatorRetryStaysWithInstance(testPage, {
      sourceKey: deliveryLead.key,
      sourceName: "Delivery lead",
      targetName: "Reviewer",
      gesture: "click",
    });

    const picker = testPage.getByTestId("managed-chat-instance-picker");
    await picker.click();
    await testPage.getByRole("dialog").getByRole("button", { name: "Delivery lead" }).click();
    await expect(picker).toContainText("Delivery lead");

    await testPage.getByTestId("coordinator-delegate-title").fill("Coordinator delegated task");
    await testPage
      .getByTestId("coordinator-delegate-description")
      .fill("Follow up with the owner and report the result.");
    await testPage.getByTestId("coordinator-delegate-submit").click();
    await expect(testPage.getByTestId("coordinator-task-list")).toContainText(
      "Coordinator delegated task",
    );
    await expect(testPage.getByTestId("workspace-task-status")).toBeVisible();

    const disable = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${COORDINATOR_PLUGIN_ID}/disable`,
    );
    if (!disable.ok) throw new Error(`Could not disable coordinator package: ${disable.status}`);
    const enable = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${COORDINATOR_PLUGIN_ID}/enable`,
    );
    if (!enable.ok) throw new Error(`Could not re-enable coordinator package: ${enable.status}`);
    await testPage.goto(`/plugins/${COORDINATOR_PLUGIN_ID}`);
    await expect(testPage.getByTestId("coordinator-page")).toBeVisible();
    const restored = await invokeCoordinatorAction<{ instances: CoordinatorInstance[] }>(
      apiClient,
      seedData.workspaceId,
      "instance.list",
    );
    expect(restored.instances.map((instance) => instance.name).sort()).toEqual([
      "Delivery lead",
      "Reviewer",
    ]);
  });
});
