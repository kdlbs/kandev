import { test } from "../../fixtures/test-base";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
  checkExecutorSandboxSettings,
  checkExecutorWorkspacePolicies,
} from "../../helpers/execution-settings";

test("executor settings save an explicit provider, ceiling and default", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const executors = (await apiClient.listExecutors()).executors;
  const executor = executors.find((item) => item.type === "local");
  if (!executor) throw new Error("Local executor required");
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Sandbox executor settings",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  try {
    const fixture = await installSandboxSettingsFixture(testPage, "unused", executors, {
      workspaceId: seedData.workspaceId,
      workspacePolicies: true,
    });
    await testPage.goto(`/settings/executors/${profile.id}`);
    await enableSandboxSettingsFixture(testPage);
    await checkExecutorSandboxSettings(
      testPage,
      fixture.executorWrites,
      false,
      fixture.policyWrites,
    );
    await checkExecutorWorkspacePolicies(testPage, fixture.executorWrites, false);
    await prCapture.screenshot("workspace-policy-desktop", {
      caption: "Make a workspace policy eligible while keeping installation defaults and ceilings.",
    });
  } finally {
    await apiClient.deleteExecutorProfile(profile.id);
  }
});
