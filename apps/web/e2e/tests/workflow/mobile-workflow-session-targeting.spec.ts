import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { createWorkflowAgentProfiles } from "./workflow-agent-switch-helpers";

test.describe("mobile: workflow session targeting", () => {
  test("keeps lifecycle controls fixed while choosing and saving a target", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const { profileA } = await createWorkflowAgentProfiles(apiClient);
    const { agents } = await apiClient.listAgents();
    const agentId = agents.find((agent) => agent.id !== "dynamic")?.id;
    if (!agentId) test.fail(true, "the E2E fixture has no launchable agent family");
    for (let index = 0; index < 10; index++) {
      await apiClient.createAgentProfile(agentId!, `Long list ${index}`, { model: "mock-fast" });
    }

    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Mobile Session Targets");
    await apiClient.createWorkflowStep(workflow.id, "Plan", 0, { is_start_step: true });
    const implement = await apiClient.createWorkflowStep(workflow.id, "Implement", 1);
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 2);
    await apiClient.updateWorkflowStep(implement.id, { agent_profile_id: profileA.id });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard("Mobile Session Targets");
    await settings.selectStep(card, "Review", true);

    const selector = settings.stepAgentProfileSelect(card);
    await selector.tap();
    const picker = testPage.getByTestId(`${review.id}-profile-picker-content`);
    await expect(picker).toBeVisible();
    const lifecycle = testPage.getByTestId(`${review.id}-profile-session-lifecycle-select`);
    await expect(lifecycle).toBeVisible();
    const lifecycleBox = await lifecycle.boundingBox();
    expect(lifecycleBox).not.toBeNull();
    expect(lifecycleBox!.height).toBeGreaterThanOrEqual(44);
    expect(lifecycleBox!.x).toBeGreaterThanOrEqual(0);
    expect(lifecycleBox!.x + lifecycleBox!.width).toBeLessThanOrEqual(390);
    expect(
      await lifecycle.evaluate(
        (element, contentId) => element.closest(`[data-testid="${contentId}"]`) === null,
        `${review.id}-profile-picker-content`,
      ),
    ).toBe(true);

    const search = picker.getByPlaceholder("Search agent profiles...");
    await search.fill("no mobile target");
    await expect(picker.getByText("Profile not found.", { exact: true })).toBeVisible();
    await expect(lifecycle).toBeVisible();
    await search.fill("");
    await expect(
      picker.getByTestId(`${review.id}-session-target-step-${implement.id}`),
    ).toBeVisible();

    const initialTarget = picker.getByTestId(`${review.id}-session-target-initial`);
    await initialTarget.tap();
    await expect(selector).toContainText("Initial agent session");

    await selector.tap();
    await testPage.getByTestId(`${review.id}-profile-session-lifecycle-select`).tap();
    const lifecycleContent = testPage.getByTestId(`${review.id}-profile-session-lifecycle-content`);
    const newOnStart = lifecycleContent.getByTestId(`${review.id}-profile-session-start-new`);
    const parkOnEnd = lifecycleContent.getByTestId(`${review.id}-profile-session-end-park`);
    for (const control of [newOnStart, parkOnEnd]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(390);
    }
    await newOnStart.tap();
    await parkOnEnd.tap();
    await testPage.getByRole("button", { name: "Back", exact: true }).tap();
    await testPage.keyboard.press("Escape");
    await expect(selector).toBeFocused();

    await settings.saveChanges(true);
    const savedReview = (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
      (step) => step.id === review.id,
    );
    expect(savedReview).toMatchObject({
      session_target: { kind: "initial" },
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
    });
    expect(savedReview?.agent_profile_id ?? "").toBe("");

    await settings.goto(seedData.workspaceId);
    const reloadedCard = await settings.findWorkflowCard("Mobile Session Targets");
    await settings.selectStep(reloadedCard, "Review", true);
    await expect(settings.stepAgentProfileSelect(reloadedCard)).toContainText(
      "Initial agent session",
    );
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
