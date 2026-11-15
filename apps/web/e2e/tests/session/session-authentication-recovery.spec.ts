import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  createSessionAuthenticationRecoveryFixture,
  SESSION_AUTHENTICATION_PRIVACY_SENTINEL,
} from "../../helpers/session-authentication-recovery";
import { SessionPage } from "../../pages/session-page";

test("retains ACP authentication evidence and retries after fixture login", async ({
  testPage,
  apiClient,
  backend,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  const fixture = await createSessionAuthenticationRecoveryFixture(
    apiClient,
    backend,
    seedData,
    `Session authentication recovery ${Date.now()}`,
  );
  try {
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    const first = await fixture.currentFailure();
    expect(first.causes).toContainEqual(
      expect.objectContaining({ code: "authentication_required" }),
    );
    const beforeLogin = await apiClient.wsRequest<{ acp_session_id?: string }>(
      "task.session.status",
      { task_id: fixture.taskId, session_id: fixture.sessionId },
    );
    expect(beforeLogin.acp_session_id).toBeUndefined();

    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expectAuthenticationCard(session);
    if (prCapture.capturing) {
      const recovery = session.activeChat().getByTestId("session-recovery-card");
      await waitForFiniteAnimations(recovery);
      await prCapture.screenshot("desktop-authentication-recovery", {
        caption: "Provider sign-in guidance, safe retry choices, and copy-only terminal command",
        fullPage: true,
      });
    }

    const details = session
      .activeChat()
      .getByTestId("session-recovery-card")
      .locator("details")
      .first();
    await details.locator("summary").click();
    await expect(details).toContainText("Cause: authentication_required");
    await expect(session.activeChat().getByTestId("session-recovery-card")).not.toContainText(
      SESSION_AUTHENTICATION_PRIVACY_SENTINEL,
    );
    await details.getByRole("button", { name: "Copy details" }).click();
    await expect
      .poll(() => testPage.evaluate(() => navigator.clipboard.readText()))
      .toContain("Cause: authentication_required");

    await testPage.goto("/settings");
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await testPage.reload();
    await session.waitForLoad();
    await expectAuthenticationCard(session);

    fixture.completeLogin();
    const retry = session.activeChat().getByTestId("session-recovery-card");
    await retry.getByTestId("recovery-fresh-button").click();
    await expect.poll(() => fixture.loginObservations(), { timeout: 30_000 }).toContain(true);
    await expect.poll(() => fixture.promptRequestCount(), { timeout: 30_000 }).toBeGreaterThan(0);
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
          return messages.some(
            (message) =>
              message.author_type === "agent" &&
              message.content.includes("Authenticated ACP fixture is ready"),
          );
        },
        { timeout: 60_000 },
      )
      .toBe(true);
    await expect(session.activeChat()).toContainText("Authenticated ACP fixture is ready");
    await expect(session.activeChat().getByTestId("session-recovery-card")).toBeHidden();
    const afterLogin = await apiClient.wsRequest<{ acp_session_id?: string }>(
      "task.session.status",
      { task_id: fixture.taskId, session_id: fixture.sessionId },
    );
    expect(afterLogin.acp_session_id).toBe("cursor-auth-fixture-session");
    const workspaces = fixture.startedWorkspaces();
    expect(workspaces.length).toBeGreaterThanOrEqual(2);
    expect(new Set(workspaces).size).toBe(1);

    const durableData = await fixture.durableRecoveryData();
    expect(durableData).not.toContain(SESSION_AUTHENTICATION_PRIVACY_SENTINEL);
    await expect(testPage.locator("body")).not.toContainText(
      SESSION_AUTHENTICATION_PRIVACY_SENTINEL,
    );
  } finally {
    await fixture.dispose();
  }
});

async function expectAuthenticationCard(session: SessionPage) {
  const card = session.activeChat().getByTestId("session-recovery-card");
  await expect(card).toContainText(
    "Cursor authentication is required. Sign in on the execution host before retrying.",
  );
  await expect(card).toContainText("Cursor CLI login");
  await expect(card).toContainText("cursor_login");
  await expect(card.getByTestId("recovery-fresh-button")).toContainText("Restart session");
  await expect(card.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(card.getByTestId("recovery-restore-workspace-button")).toBeVisible();
  await expect(card.getByRole("button", { name: "Open terminal" })).toHaveCount(0);
  await expect(card).toContainText("cursor-agent login");
  await expect(card.getByRole("button", { name: "Copy command" })).toBeVisible();
  await expect(card.getByRole("button", { name: "Run in terminal" })).toHaveCount(0);
}
