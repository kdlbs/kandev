import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * E2E coverage for the Office workspace kill switch
 * (docs/specs/office/requirements/workspace-kill-switch.md), per the system
 * design's E2E decision: pause from the banner, the banner surviving an
 * Office navigation, a blocked launch, and resume — in one flow so the
 * shared worker-scoped workspace is never left paused for a later test.
 */
test.describe("Office workspace kill switch", () => {
  test.afterEach(async ({ officeApi, officeSeed }) => {
    // Best-effort: this workspace is shared by every test in the worker.
    await officeApi.resumeWorkspace(officeSeed.workspaceId).catch(() => undefined);
  });

  test("pause blocks a manual launch across navigation, resume restores it", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "Kill switch routine",
      task_template: JSON.stringify({ title: "Kill switch check", description: "e2e" }),
    })) as { id: string };

    // Baseline: a manual fire succeeds while the workspace is running.
    const baselineFire = await officeApi.runRoutine(routine.id);
    expect(baselineFire.status).toBe(200);

    await testPage.goto(`/office?workspaceId=${officeSeed.workspaceId}`);
    await expect(testPage.getByTestId("office-workspace-running-bar")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByTestId("office-workspace-paused-banner")).toHaveCount(0);

    // Pause from the banner (AC-OFFICE-KILL-SWITCH-006.12): a reason and an
    // explicit confirmation are both required before the request is sent.
    await testPage.getByTestId("office-pause-workspace-button").click();
    const pauseDialog = testPage.getByTestId("office-pause-workspace-dialog");
    await expect(pauseDialog).toBeVisible();
    const confirmButton = pauseDialog.getByTestId("office-pause-confirm-button");
    await expect(confirmButton).toBeDisabled();
    await pauseDialog.getByTestId("office-pause-reason-input").fill("E2E kill switch drill");
    await expect(confirmButton).toBeEnabled();

    const pausePosted = waitForHttp(testPage, "POST", /\/office\/workspaces\/[^/]+\/pause$/);
    await confirmButton.click();
    await pausePosted;

    // AC-OFFICE-KILL-SWITCH-006.4: the indicator names actor, reason and time.
    const banner = testPage.getByTestId("office-workspace-paused-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("E2E kill switch drill");
    await expect(banner).toContainText("default-user"); // AC-004.7 single-user sentinel

    // The banner persists across an Office navigation (AC-006.4 "every page").
    await testPage.goto(`/office/inbox`);
    await expect(testPage.getByTestId("office-workspace-paused-banner")).toBeVisible();
    await expect(testPage.getByTestId("office-workspace-paused-banner")).toContainText(
      "E2E kill switch drill",
    );

    // Blocked launch (AC-OFFICE-KILL-SWITCH-002.4): a manual fire is
    // rejected with 409 and dispatches nothing while paused.
    const blockedFire = await officeApi.runRoutine(routine.id);
    expect(blockedFire.status).toBe(409);
    const blockedBody = (await blockedFire.json()) as { paused: boolean };
    expect(blockedBody.paused).toBe(true);

    // Resume requires explicit confirmation (AC-006.5), no reason needed.
    await testPage.getByTestId("office-resume-workspace-button").click();
    const resumeDialog = testPage.getByTestId("office-resume-workspace-dialog");
    await expect(resumeDialog).toBeVisible();

    const resumePosted = waitForHttp(testPage, "POST", /\/office\/workspaces\/[^/]+\/resume$/);
    await resumeDialog.getByTestId("office-resume-confirm-button").click();
    await resumePosted;

    await expect(testPage.getByTestId("office-workspace-paused-banner")).toHaveCount(0);
    await expect(testPage.getByTestId("office-workspace-running-bar")).toBeVisible();

    // Resume is effective (AC-005.2/-005.3): the same routine can fire again.
    const resumedFire = await officeApi.runRoutine(routine.id);
    expect(resumedFire.status).toBe(200);
  });

  test("refresh corrects a stale client after another caller changes pause state", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await testPage.goto(`/office?workspaceId=${officeSeed.workspaceId}`);
    await expect(testPage.getByTestId("office-workspace-running-bar")).toBeVisible({
      timeout: 10_000,
    });

    // Another caller (an API client, not this page) pauses the workspace.
    const pauseRes = await officeApi.pauseWorkspace(officeSeed.workspaceId, "Out-of-band pause");
    expect(pauseRes.status).toBe(200);

    // AC-006.13: refresh re-reads and displays what the server holds.
    const refreshRead = waitForHttp(testPage, "GET", /\/office\/workspaces\/[^/]+\/pause$/);
    await testPage.getByTestId("office-pause-refresh-running").click();
    await refreshRead;

    const banner = testPage.getByTestId("office-workspace-paused-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("Out-of-band pause");
  });
});
