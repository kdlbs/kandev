import { expect, test } from "../../fixtures/test-base";
import {
  agentStartupProcessPrivacySentinels,
  createAgentStartupProcessFixture,
} from "../../helpers/agent-startup-process-diagnostics";
import { SessionPage } from "../../pages/session-page";

test("shows the confirmed mise startup refusal and preserves distinct failure history", async ({
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
    `Agent startup diagnostics ${Date.now()}`,
  );
  try {
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    const first = await fixture.currentFailure();
    expect(first.reason).toBe("mise_config_untrusted");
    expect(first.exitCode).toBe(1);

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

    const details = recovery.locator("details").first();
    await expect(details).not.toHaveAttribute("open", "");
    await details.locator("summary").click();
    await expect(details).toContainText("Cause: process_exited");
    await expect(details).toContainText("Reason: mise_config_untrusted");
    await expect(details).toContainText("Exit code: 1");
    for (const sentinel of agentStartupProcessPrivacySentinels) {
      await expect(recovery).not.toContainText(sentinel);
    }
    await details.getByRole("button", { name: "Copy details" }).click();
    await expect
      .poll(() => testPage.evaluate(() => navigator.clipboard.readText()))
      .toContain("Exit code: 1");

    await testPage.reload();
    await session.waitForLoad();
    const reloaded = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      reloaded.getByRole("heading", { name: "Agent startup blocked by mise" }),
    ).toBeVisible();
    await expect(reloaded).toContainText("mise did not trust this workspace's configuration.");
    await expect(reloaded.getByTestId("recovery-fresh-button")).toBeVisible();

    fixture.setMode("generic");
    const distinct = await clickFreshStartAndAwaitFailure(
      session,
      fixture,
      first.stamp,
      "unclassified",
    );
    expect(distinct.exitCode).toBe(3);
    const current = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      current.getByRole("heading", { name: "Agent exited before startup" }),
    ).toBeVisible();
    await expect(current).toContainText("The agent exited before startup completed.");
    const distinctDetails = current.locator("details").first();
    await distinctDetails.locator("summary").click();
    await expect(distinctDetails).toContainText("Exit code: 3");
    await expect(session.activeChat()).toContainText(
      "mise did not trust this workspace's configuration.",
    );
  } finally {
    await fixture.dispose();
  }
});

async function clickFreshStartAndAwaitFailure(
  session: SessionPage,
  fixture: Awaited<ReturnType<typeof createAgentStartupProcessFixture>>,
  previousStamp: string,
  reason: string,
) {
  const card = session.activeChat().getByTestId("session-recovery-card");
  await card.getByTestId("recovery-fresh-button").click();
  return fixture.waitForNewFailure(previousStamp, reason);
}
