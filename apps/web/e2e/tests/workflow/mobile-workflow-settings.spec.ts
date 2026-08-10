import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings on mobile", () => {
  test("configures original-session rules with touch-sized controls", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Session Settings",
    );
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Work", 0, {
      is_start_step: true,
    });

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);
    const card = await page.findWorkflowCard("Mobile Session Settings");
    await expect(card).toBeVisible();
    await page.selectStep(card, "Work", true);

    const agentProfileHelpId = `${workStep.id}-agent-profile-help`;
    const originalSessionHelpId = `${workStep.id}-override-original-session-help`;
    const helpOrder = await card
      .locator(`[data-testid="${agentProfileHelpId}"], [data-testid="${originalSessionHelpId}"]`)
      .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-testid")));
    expect(helpOrder).toEqual([agentProfileHelpId, originalSessionHelpId]);

    await card.getByLabel("Override original session options").tap();
    const editor = card.getByTestId(`${workStep.id}-session-config-editor`);
    const rule = editor.getByTestId("session-config-rule-0");
    await expect(rule).toBeVisible();

    const settings = rule.getByRole("button", { name: /Settings for/ });
    await settings.tap();
    await testPage.getByText("Mock Smart", { exact: true }).tap();
    await testPage.getByTestId("config-option-trigger-effort").tap();
    await testPage.getByRole("button", { name: "Max", exact: true }).tap();
    await testPage.keyboard.press("Escape");
    await prCapture.screenshot("mobile-original-session-editor", {
      caption: "Mobile workflow step editor with touch-sized original-session controls.",
    });

    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    for (const control of [editor, rule, settings]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
    }
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);

    await page.saveChanges(true);
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    expect(steps.find((step) => step.id === workStep.id)?.events?.on_enter?.[0]).toMatchObject({
      type: "configure_session",
      config: {
        rules: [
          {
            operation: "set",
            model: "mock-smart",
            config_options: { effort: "max" },
          },
        ],
      },
    });
  });

  test("configures an all child tasks complete transition", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Child Completion Settings",
    );
    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    await apiClient.createWorkflowStep(workflow.id, "Done", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Mobile Child Completion Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Waiting").click();

    const childCompletionSelect = card.getByTestId(
      `${waitStep.id}-children-completed-transition-select`,
    );
    await expect(childCompletionSelect).toBeVisible();
    await childCompletionSelect.click();
    await testPage.getByRole("option", { name: "Move to next step" }).click();

    const beforeSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      beforeSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toBeUndefined();

    const floatingSave = page.floatingSave;
    await expect(floatingSave).toBeVisible();
    const saveButton = floatingSave.getByRole("button", { name: "Save changes" });
    const saveBox = await saveButton.boundingBox();
    expect(saveBox).not.toBeNull();
    expect(saveBox!.height).toBeGreaterThanOrEqual(44);
    await page.saveChanges();
    const afterSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      afterSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toEqual([{ type: "move_to_next" }]);

    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const editorControls = [
      card.getByPlaceholder("Step name"),
      childCompletionSelect,
      card.getByRole("button", { name: "Delete", exact: true }),
    ];
    for (const control of editorControls) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
    }
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });

  test("keeps workflow controls within the mobile viewport", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    const nameInput = card.locator("input").first();
    await nameInput.fill("Mobile Workflow Draft");
    await expect(page.floatingSave).toBeVisible();
    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty", "true");

    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const controls = [
      page.addWorkflowButton,
      card.locator("input").first(),
      page.workflowAgentProfileSelect(card),
      page.deleteWorkflowButton(card),
      page.floatingSave.getByRole("button", { name: "Save changes" }),
    ];
    for (const control of controls) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
    }

    const hasDocumentOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(hasDocumentOverflow).toBe(false);
  });
});
