import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  agentStartupProcessPrivacySentinels,
  createAgentStartupProcessFixture,
} from "../../helpers/agent-startup-process-diagnostics";
import { SessionPage } from "../../pages/session-page";

test("keeps mise startup recovery touch-safe with distinct history on phone", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.skip(process.platform === "win32", "the disposable agent fixture is a POSIX script");
  test.setTimeout(180_000);

  const fixture = await createAgentStartupProcessFixture(
    apiClient,
    backend,
    seedData,
    `Mobile agent startup diagnostics ${Date.now()}`,
  );
  try {
    const first = await fixture.currentFailure();
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const recovery = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      recovery.getByRole("heading", { name: "Agent startup blocked by mise" }),
    ).toBeVisible();
    await expect(recovery).toContainText("mise did not trust this workspace's configuration.");
    await expect(recovery.getByTestId("session-recovery-prerequisite")).toContainText(
      "Review the configuration on the execution host",
    );

    const freshStart = recovery.getByTestId("recovery-fresh-button");
    await expect(freshStart).toBeVisible();
    await expect(freshStart).toBeInViewport();
    const freshBox = await freshStart.boundingBox();
    expect(freshBox).not.toBeNull();
    expect(freshBox!.height).toBeGreaterThanOrEqual(44);

    const details = recovery.locator("details").first();
    await details.locator("summary").tap();
    await expect(details).toContainText("Cause: process_exited");
    await expect(details).toContainText("Reason: mise_config_untrusted");
    await expect(details).toContainText("Exit code: 1");
    for (const sentinel of agentStartupProcessPrivacySentinels) {
      await expect(recovery).not.toContainText(sentinel);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mise startup recovery on phone");

    await testPage.reload();
    await session.waitForLoad();
    const reloaded = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      reloaded.getByRole("heading", { name: "Agent startup blocked by mise" }),
    ).toBeVisible();
    const reloadedFresh = reloaded.getByTestId("recovery-fresh-button");
    const reloadedBox = await reloadedFresh.boundingBox();
    expect(reloadedBox).not.toBeNull();
    expect(reloadedBox!.height).toBeGreaterThanOrEqual(44);

    fixture.setMode("generic");
    await reloadedFresh.tap();
    const distinct = await fixture.waitForNewFailure(first.stamp, "unclassified");
    expect(distinct.exitCode).toBe(3);
    const current = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      current.getByRole("heading", { name: "Agent exited before startup" }),
    ).toBeVisible();
    await expect(session.activeChat()).toContainText(
      "mise did not trust this workspace's configuration.",
    );
    await assertNoDocumentHorizontalOverflow(testPage, "distinct startup failure on phone");
  } finally {
    await fixture.dispose();
  }
});
