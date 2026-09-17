import { test, expect } from "../../fixtures/test-base";
import { resetWorkspaceOrchestrators } from "../../helpers/orchestration";
import { AutomationsPage } from "../../pages/automations-page";

test("daily automation delivers to its workspace orchestrator and links the reply", async ({
  testPage,
  backend,
  seedData,
}) => {
  await backend.restart({ KANDEV_FEATURES_ORCHESTRATION: "true", KANDEV_FEATURES_OFFICE: "false" });
  await resetWorkspaceOrchestrators(testPage.request, backend.baseUrl, seedData.workspaceId);
  const ws = seedData.workspaceId;
  await testPage.goto(`/settings/workspaces/${ws}/orchestration/new`);
  await testPage.getByTestId("orchestrator-profile").click();
  await testPage.getByRole("option").first().click();
  await testPage.getByTestId("persona-executor-profile").click();
  await testPage.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await testPage.getByRole("button", { name: "Add orchestrator", exact: true }).click();
  await expect(testPage).not.toHaveURL(/\/new$/);
  const chief = new URL(testPage.url()).pathname.split("/").pop()!;
  const automations = new AutomationsPage(testPage, ws);
  await automations.gotoNew();
  await automations.nameInput.fill("Daily assigned PR review");
  await automations.selectFrequency("every day");
  await testPage.getByTestId("automation-target").click();
  await testPage.getByRole("option", { name: "Chief of staff", exact: true }).click();
  await expect(testPage.getByTestId("workflow-selector")).toHaveCount(0);
  const promptEditor = testPage.getByRole("textbox", { name: "Prompt editor", exact: true });
  await promptEditor.focus();
  await promptEditor.press("ControlOrMeta+A");
  await testPage.keyboard.insertText(
    "Check open PRs assigned to me and summarize what needs to move forward.",
  );
  await automations.saveButton.click();
  await expect(testPage).toHaveURL(/automations$/);
  const row = automations.table.locator("tr", { hasText: "Daily assigned PR review" });
  const automationId = (await row.getAttribute("data-testid"))!.replace("automation-row-", "");
  await testPage.goto(`/automations/${automationId}`);
  await testPage.getByRole("button", { name: "Run now", exact: true }).click();
  await expect(testPage.getByTestId("automation-orchestrator-delivery")).toBeVisible();
  await expect(
    testPage.getByText("Delivered to orchestrator", { exact: true }).first(),
  ).toBeVisible();
  await testPage.getByRole("link", { name: "Open orchestrator chat", exact: true }).click();
  await expect(testPage.getByTestId("orchestrator-conversation")).toBeVisible();
  const conversation = new URL(testPage.url()).pathname.split("/").pop()!;
  const commentsUrl = `${backend.baseUrl}/api/v1/orchestration/tasks/${conversation}/comments`;
  await expect
    .poll(
      async () => {
        const data = await (await testPage.request.get(commentsUrl)).json();
        return data.comments.filter((comment: { source: string }) => comment.source === "session")
          .length;
      },
      { timeout: 60000 },
    )
    .toBe(1);
  const config = await (
    await testPage.request.get(
      `${backend.baseUrl}/api/v1/orchestration/workspaces/${ws}/orchestrators/${chief}`,
    )
  ).json();
  expect(config.profile_id).toBeTruthy();
  await testPage.goto(`/settings/workspaces/${ws}/automations/${automationId}`);
  await expect(testPage.getByTestId("automation-target")).toContainText("Chief of staff");
  await expect(automations.frequencySelector).toContainText("every day");
});
