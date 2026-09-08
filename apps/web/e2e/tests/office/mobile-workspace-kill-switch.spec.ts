// Filename starts with "mobile-" so this runs on the mobile-chrome project (Pixel 5, touch).
import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

/**
 * AC-OFFICE-KILL-SWITCH-006.8: on a 390x844 phone viewport the paused
 * indicator renders without clipping actor, reason or time, and the pause,
 * resume and confirmation controls stay reachable and operable — the same
 * capability as on desktop (workspace-kill-switch.spec.ts).
 */
test.describe("mobile: Office workspace kill switch", () => {
  test.afterEach(async ({ officeApi, officeSeed }) => {
    await officeApi.resumeWorkspace(officeSeed.workspaceId).catch(() => undefined);
  });

  test("pause and resume controls are reachable and unclipped on a phone viewport", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });

    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "Mobile kill switch routine",
      task_template: JSON.stringify({ title: "Mobile kill switch check", description: "e2e" }),
    })) as { id: string };

    await testPage.goto(`/office?workspaceId=${officeSeed.workspaceId}`);
    await expect(testPage.getByTestId("office-workspace-running-bar")).toBeVisible({
      timeout: 10_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage);

    // Pause control is reachable and operable on a phone viewport (AC-006.12, -006.8).
    await testPage.getByTestId("office-pause-workspace-button").tap();
    const pauseDialog = testPage.getByTestId("office-pause-workspace-dialog");
    await expect(pauseDialog).toBeVisible();
    await pauseDialog.getByTestId("office-pause-reason-input").fill("Mobile E2E kill switch drill");

    const pausePosted = waitForHttp(testPage, "POST", /\/office\/workspaces\/[^/]+\/pause$/);
    await pauseDialog.getByTestId("office-pause-confirm-button").tap();
    await pausePosted;

    const banner = testPage.getByTestId("office-workspace-paused-banner");
    await expect(banner).toBeVisible();
    // Actor, reason and time render without being clipped off-screen.
    await expect(banner).toContainText("Mobile E2E kill switch drill");
    await expect(banner).toContainText("default-user");
    await assertNoDocumentHorizontalOverflow(testPage);

    // Banner persists across an Office navigation on mobile too.
    await testPage.goto(`/office/inbox`);
    await expect(testPage.getByTestId("office-workspace-paused-banner")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    // Blocked launch while paused (AC-002.4).
    const blockedFire = await officeApi.runRoutine(routine.id);
    expect(blockedFire.status).toBe(409);

    // Resume requires explicit confirmation and is operable via touch (AC-006.5, -006.8).
    await testPage.getByTestId("office-resume-workspace-button").tap();
    const resumeDialog = testPage.getByTestId("office-resume-workspace-dialog");
    await expect(resumeDialog).toBeVisible();

    const resumePosted = waitForHttp(testPage, "POST", /\/office\/workspaces\/[^/]+\/resume$/);
    await resumeDialog.getByTestId("office-resume-confirm-button").tap();
    await resumePosted;

    await expect(testPage.getByTestId("office-workspace-paused-banner")).toHaveCount(0);
    await expect(testPage.getByTestId("office-workspace-running-bar")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    const resumedFire = await officeApi.runRoutine(routine.id);
    expect(resumedFire.status).toBe(200);
  });
});
