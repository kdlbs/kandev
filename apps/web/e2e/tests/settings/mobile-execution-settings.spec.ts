import { test, expect } from "../../fixtures/test-base";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
  checkSandboxSettingsSelection,
} from "../../helpers/execution-settings";

test("phone Execution settings use policy sheets without losing the draft", async ({
  testPage,
  apiClient,
}, testInfo) => {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("mock-agent is required");
  const profile = await apiClient.createAgentProfile(agent.id, "Phone execution settings", {
    model: "mock-fast",
    command_prefix: "env --",
  });
  try {
    const fixture = await installSandboxSettingsFixture(
      testPage,
      profile.id,
      (await apiClient.listExecutors()).executors,
    );
    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await expect(testPage.getByTestId("command-prefix-input")).toHaveValue("env --");
    await enableSandboxSettingsFixture(testPage);
    await checkSandboxSettingsSelection(testPage, fixture.writes, true, fixture.contexts);
    await testPage
      .getByTestId("agent-sandbox-settings")
      .screenshot({ path: testInfo.outputPath("execution-settings.png") });
  } finally {
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
