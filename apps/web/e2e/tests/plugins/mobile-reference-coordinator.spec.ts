import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  installAndGrantCoordinator,
  invokeCoordinatorAction,
  uninstallCoordinator,
  verifyCoordinatorRetryStaysWithInstance,
} from "./reference-coordinator-helpers";

test.describe("reference coordinator on phone", () => {
  let alternateProfileId = "";

  test.afterEach(async ({ apiClient }) => {
    await uninstallCoordinator(apiClient);
    if (alternateProfileId) {
      await apiClient.deleteAgentProfile(alternateProfileId, true).catch(() => undefined);
      alternateProfileId = "";
    }
  });

  test("phone instance selection and settings", async ({ testPage, apiClient, seedData }) => {
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
      `Coordinator phone ${randomUUID().slice(0, 8)}`,
      {
        model: baseProfile.model,
        auto_approve: true,
      },
    );
    alternateProfileId = alternateProfile.id;

    await testPage.goto("/plugins/kandev-plugin-coordinator");
    await testPage.getByTestId("coordinator-add-instance").tap();
    await expect(testPage.getByTestId("coordinator-settings")).toBeVisible();
    await testPage.getByTestId("coordinator-field-name").fill("Delivery lead");
    await testPage
      .getByTestId("coordinator-field-agent-profile")
      .selectOption(seedData.agentProfileId);
    await testPage
      .getByTestId("coordinator-field-instructions")
      .fill("Coordinate only approved delivery work.");
    await testPage.getByTestId("coordinator-save").tap();
    await expect(testPage.getByTestId("coordinator-page")).toBeVisible();

    await testPage.getByTestId("coordinator-add-instance").tap();
    await expect(testPage.getByTestId("coordinator-settings")).toBeVisible();
    await testPage.getByTestId("coordinator-field-name").fill("Reviewer");
    await testPage.getByTestId("coordinator-field-agent-profile").selectOption(alternateProfileId);
    await testPage
      .getByTestId("coordinator-field-instructions")
      .fill("Review changes and report verified findings.");
    await testPage.getByTestId("coordinator-save").tap();
    await expect(testPage.getByTestId("coordinator-page")).toBeVisible();

    const picker = testPage.getByTestId("managed-chat-instance-picker");
    await expect
      .poll(async () => (await picker.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await picker.tap();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    await drawer.getByRole("button", { name: "Delivery lead" }).tap();
    await expect(picker).toContainText("Delivery lead");

    const listed = await invokeCoordinatorAction<{
      instances: Array<{ key: string; name: string }>;
    }>(apiClient, seedData.workspaceId, "instance.list");
    const deliveryLead = listed.instances.find((instance) => instance.name === "Delivery lead");
    if (!deliveryLead) throw new Error("Delivery lead coordinator instance was not returned");
    await verifyCoordinatorRetryStaysWithInstance(testPage, {
      sourceKey: deliveryLead.key,
      sourceName: "Delivery lead",
      targetName: "Reviewer",
      gesture: "tap",
    });

    const chatPause = testPage.getByRole("button", { name: "Pause" });
    const pauseResponses: string[] = [];
    testPage.on("response", async (response) => {
      if (
        response.url().endsWith("/api/plugins/kandev-plugin-coordinator/actions/conversation.pause")
      ) {
        pauseResponses.push(`${response.status()}: ${await response.text()}`);
      }
    });
    await chatPause.tap();
    await expect(testPage.getByRole("button", { name: "Resume" })).toBeEnabled();
    await testPage.getByRole("button", { name: "Resume" }).tap();
    await expect.poll(() => pauseResponses.length).toBe(2);
    await expect(testPage.getByRole("button", { name: "Pause" })).toBeEnabled();

    await testPage.getByTestId("coordinator-open-settings").tap();
    await expect(testPage.getByTestId("coordinator-settings")).toBeVisible();
    await expect(testPage.getByTestId("coordinator-field-agent-profile")).toHaveValue(
      seedData.agentProfileId,
    );
    await expect(testPage.getByTestId("coordinator-field-instructions")).toHaveValue(
      "Coordinate only approved delivery work.",
    );
    const pageGeometry = await testPage.locator("body").evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(pageGeometry.scrollWidth).toBeLessThanOrEqual(pageGeometry.clientWidth + 1);
  });
});
