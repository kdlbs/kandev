import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  createSessionAuthenticationRecoveryFixture,
  SESSION_AUTHENTICATION_PRIVACY_SENTINEL,
} from "../../helpers/session-authentication-recovery";
import { SessionPage } from "../../pages/session-page";

test("keeps authentication recovery touch-safe and pending on phone", async ({
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
    `Mobile session authentication recovery ${Date.now()}`,
  );
  try {
    const first = await fixture.currentFailure();
    expect(first.causes).toContainEqual(
      expect.objectContaining({ code: "authentication_required" }),
    );
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const recovery = session.activeChat().getByTestId("session-recovery-card");
    await expect(recovery).toContainText(
      "Cursor authentication is required. Sign in on the execution host before retrying.",
    );
    await expect(recovery).toContainText("Cursor CLI login");
    await expect(recovery).toContainText("cursor_login");
    await expect(recovery.getByRole("button", { name: "Open terminal" })).toHaveCount(0);
    await expect(recovery).toContainText("cursor-agent login");
    await expect(recovery.getByRole("button", { name: "Copy command" })).toBeVisible();
    await expect(recovery.getByRole("button", { name: "Run in terminal" })).toHaveCount(0);
    await expect(recovery.getByTestId("recovery-restore-workspace-button")).toBeVisible();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(recovery);
      await prCapture.screenshot("mobile-authentication-recovery", {
        caption: "Touch-safe sign-in guidance, retry, and copy-only terminal command",
        fullPage: true,
      });
    }

    const restart = recovery.getByTestId("recovery-fresh-button");
    await expect(restart).toBeVisible();
    const restartBounds = await restart.boundingBox();
    expect(restartBounds).not.toBeNull();
    expect(restartBounds!.height).toBeGreaterThanOrEqual(44);

    const details = recovery.locator("details").first();
    await details.locator("summary").tap();
    await expect(details).toContainText("Cause: authentication_required");
    await expect(recovery).not.toContainText(SESSION_AUTHENTICATION_PRIVACY_SENTINEL);
    await assertNoDocumentHorizontalOverflow(testPage, "ACP authentication recovery on phone");

    fixture.completeLogin();
    fixture.holdNextSessionStart();
    await restart.tap();
    await fixture.waitForHeldSessionStart();
    await expect(restart).toBeDisabled();
    await expect(recovery.getByRole("status").filter({ hasText: "Starting..." })).toBeVisible();
    fixture.releaseHeldSessionStart();

    await expect(session.activeChat()).toContainText("Authenticated ACP fixture is ready", {
      timeout: 60_000,
    });
    await expect(session.activeChat().getByTestId("session-recovery-card")).toBeHidden();
    await expect(session.activeChat().locator('[contenteditable="true"]').first()).toBeFocused();
    expect(fixture.startedWorkspaces().length).toBeGreaterThanOrEqual(2);
    expect(new Set(fixture.startedWorkspaces()).size).toBe(1);
    await assertNoDocumentHorizontalOverflow(testPage, "authenticated ACP recovery on phone");
    const durableData = await fixture.durableRecoveryData();
    expect(durableData).not.toContain(SESSION_AUTHENTICATION_PRIVACY_SENTINEL);
  } finally {
    fixture.releaseHeldSessionStart();
    await fixture.dispose();
  }
});
