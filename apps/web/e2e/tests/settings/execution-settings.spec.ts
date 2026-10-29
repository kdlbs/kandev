import { test, expect } from "../../fixtures/test-base";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
  checkSandboxSettingsSelection,
} from "../../helpers/execution-settings";

test("Execution settings preserve wrappers and use generic policy selections", async ({
  testPage,
  apiClient,
}) => {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("mock-agent is required");
  const profile = await apiClient.createAgentProfile(agent.id, "Execution settings", {
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
    await checkSandboxSettingsSelection(testPage, fixture.writes, false, fixture.contexts);
  } finally {
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
