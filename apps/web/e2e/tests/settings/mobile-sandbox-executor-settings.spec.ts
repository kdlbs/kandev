import { test } from "../../fixtures/test-base";
import {
  installSandboxSettingsFixture,
  enableSandboxSettingsFixture,
  checkExecutorSandboxSettings,
} from "../../helpers/execution-settings";

test("phone executor settings use sheets and save the same policy contract", async ({
  testPage,
  apiClient,
}) => {
  const executors = (await apiClient.listExecutors()).executors;
  const executor = executors.find((item) => item.type === "local");
  if (!executor) throw new Error("Local executor required");
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Phone sandbox executor settings",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  try {
    const fixture = await installSandboxSettingsFixture(testPage, "unused", executors);
    await testPage.goto(`/settings/executors/${profile.id}`);
    await enableSandboxSettingsFixture(testPage);
    await checkExecutorSandboxSettings(
      testPage,
      fixture.executorWrites,
      true,
      fixture.policyWrites,
    );
  } finally {
    await apiClient.deleteExecutorProfile(profile.id);
  }
});
