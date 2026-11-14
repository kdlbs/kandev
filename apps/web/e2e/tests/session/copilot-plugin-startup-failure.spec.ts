import { expect, test } from "../../fixtures/test-base";
import {
  copilotPluginStartupFailurePrivacySentinels,
  createCopilotPluginStartupFailureFixture,
} from "../../helpers/copilot-plugin-startup-failure";
import { SessionPage } from "../../pages/session-page";

test("keeps typed Copilot plugin startup guidance after reload", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.skip(process.platform === "win32", "the disposable npx fixture is a POSIX script");
  test.setTimeout(180_000);

  const failure = await createCopilotPluginStartupFailureFixture(
    apiClient,
    backend,
    seedData,
    `Copilot plugin startup recovery ${Date.now()}`,
  );
  test.skip(!failure, "Copilot agent is not registered in this E2E runtime");
  if (!failure) return;

  try {
    const profile = await apiClient.getAgentProfile(failure.fixture.profileId);
    expect(profile.copilotSelectedPlugins).toEqual(["installed:copilot-plugins/spark"]);

    await testPage.goto(`/t/${failure.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const recovery = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      recovery.getByRole("heading", {
        name: "The selected Copilot plugins could not be prepared",
      }),
    ).toBeVisible();
    await expect(recovery).toContainText("Reason: the plugin inventory could not be verified.");
    await expect(recovery).toContainText(`Runtime: ${failure.runtimeVersion}`);
    await expect(recovery).toContainText("Review plugin state or profile selections, then retry.");
    await expect(recovery.getByTestId("recovery-fresh-button")).toBeVisible();

    const details = recovery.locator("details").first();
    await details.locator("summary").click();
    await expect(details).toContainText(failure.runtimeVersion);
    for (const sentinel of copilotPluginStartupFailurePrivacySentinels) {
      await expect(recovery).not.toContainText(sentinel);
    }

    await testPage.reload();
    await session.waitForLoad();
    const reloaded = session.activeChat().getByTestId("session-recovery-card");
    await expect(reloaded).toContainText("the plugin inventory could not be verified");
    await expect(reloaded).toContainText(`Runtime: ${failure.runtimeVersion}`);
    await expect(reloaded).toContainText("Review plugin state or profile selections, then retry.");
    await expect(reloaded.getByTestId("recovery-fresh-button")).toBeVisible();
    for (const sentinel of copilotPluginStartupFailurePrivacySentinels) {
      await expect(reloaded).not.toContainText(sentinel);
    }
  } finally {
    await failure.dispose();
  }
});
