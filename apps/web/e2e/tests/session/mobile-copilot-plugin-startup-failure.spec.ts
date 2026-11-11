import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  copilotPluginStartupFailurePrivacySentinels,
  createCopilotPluginStartupFailureFixture,
} from "../../helpers/copilot-plugin-startup-failure";
import { SessionPage } from "../../pages/session-page";

test("keeps Copilot plugin startup recovery touch-safe after reload on phone", async ({
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
    `Mobile Copilot plugin startup recovery ${Date.now()}`,
  );
  test.skip(!failure, "Copilot agent is not registered in this E2E runtime");
  if (!failure) return;

  try {
    await testPage.goto(`/t/${failure.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const recovery = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      recovery.getByRole("heading", {
        name: "The selected Copilot plugins could not be prepared",
      }),
    ).toBeVisible();
    await expect(recovery).toContainText("the plugin inventory could not be verified");
    await expect(recovery).toContainText(`Runtime: ${failure.runtimeVersion}`);
    await expect(recovery).toContainText("Review plugin state or profile selections, then retry.");
    const freshStart = recovery.getByTestId("recovery-fresh-button");
    await expect(freshStart).toBeVisible();
    await expect(freshStart).toBeInViewport();
    const freshStartBox = await freshStart.boundingBox();
    expect(freshStartBox).not.toBeNull();
    expect(freshStartBox!.height).toBeGreaterThanOrEqual(44);

    const details = recovery.locator("details").first();
    await details.locator("summary").tap();
    await expect(details).toContainText(failure.runtimeVersion);
    for (const sentinel of copilotPluginStartupFailurePrivacySentinels) {
      await expect(recovery).not.toContainText(sentinel);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "Copilot plugin startup recovery on phone");

    await testPage.reload();
    await session.waitForLoad();
    const reloaded = session.activeChat().getByTestId("session-recovery-card");
    await expect(reloaded).toContainText("the plugin inventory could not be verified");
    await expect(reloaded).toContainText(`Runtime: ${failure.runtimeVersion}`);
    await expect(reloaded).toContainText("Review plugin state or profile selections, then retry.");
    const reloadedFreshStart = reloaded.getByTestId("recovery-fresh-button");
    const reloadedBox = await reloadedFreshStart.boundingBox();
    expect(reloadedBox).not.toBeNull();
    expect(reloadedBox!.height).toBeGreaterThanOrEqual(44);
    for (const sentinel of copilotPluginStartupFailurePrivacySentinels) {
      await expect(reloaded).not.toContainText(sentinel);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "reloaded Copilot plugin recovery on phone");
  } finally {
    await failure.dispose();
  }
});
