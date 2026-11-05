import { test, expect } from "../../fixtures/test-base";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
  checkSandboxSettingsSelection,
} from "../../helpers/execution-settings";

test("workflow policy preview preserves inherited agent selection", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("Mock agent required");
  const profile = await apiClient.createAgentProfile(agent.id, "Workflow preview", {
    model: "mock-fast",
  });
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Sandbox workflow");
  const steps = (await apiClient.listWorkflowSteps(workflow.id)).steps ?? [];
  const step = await apiClient.createWorkflowStep(workflow.id, "Sandbox review", steps.length);
  try {
    const fixture = await installSandboxSettingsFixture(
      testPage,
      step.id,
      (await apiClient.listExecutors()).executors,
      { scope: "workflow_step", agentProfileId: profile.id, workspaceId: seedData.workspaceId },
    );
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/workflows`);
    await expect(testPage.getByTestId(`workflow-step-node-${step.id}`)).toBeVisible();
    await enableSandboxSettingsFixture(testPage);
    await testPage.getByTestId(`workflow-step-node-${step.id}`).click();
    const section = testPage.getByTestId("workflow_step-sandbox-settings");
    await expect(section).toBeVisible();
    await section.getByLabel("Agent context", { exact: true }).click();
    await testPage.getByRole("option", { name: /Workflow preview/ }).click();
    await checkSandboxSettingsSelection(
      testPage,
      fixture.writes,
      testInfo.project.name === "mobile-chrome",
      fixture.contexts,
      "workflow_step",
    );
    const saved = (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
      (item) => item.id === step.id,
    );
    expect(saved?.agent_profile_id ?? "").toBe("");
  } finally {
    await apiClient.deleteWorkflow(workflow.id);
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
