// AC-COORDINATOR-NEEDS-YOU-008.1: Needs you on a phone-sized viewport
// (docs/specs/coordinator/requirements/needs-you.md, #phone-and-accessibility).
// The `mobile-chrome` Playwright project matches this filename
// (apps/web/e2e/playwright.config.ts) and applies the Pixel 5 emulation the
// assertions below rely on, per the work order's note that the 390px
// assertions live in their own file rather than a rerun of needs-you.spec.ts
// under a different project.
//
// The accessibility scan (AC .008.2) lives in mobile-needs-you-a11y.spec.ts,
// kept separate so it can be dropped independently of this file's coverage.
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

const MIN_TOUCH_TARGET_PX = 44;

async function expectNoHorizontalScroll(testPage: import("@playwright/test").Page): Promise<void> {
  const overflow = await testPage.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
}

test.describe("Coordinator screens on a phone viewport", () => {
  test("Needs you stacks in one column with 44px touch targets, no horizontal scroll and a pinned count strip (AC .008.1)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Needs You Card A",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskA.session_id) throw new Error("expected an active session for the clarification task");
    await waitForSessionState(apiClient, {
      taskId: taskA.id,
      sessionId: taskA.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session A should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Needs You Card B",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskB.session_id) throw new Error("expected an active session for the clarification task");
    await waitForSessionState(apiClient, {
      taskId: taskB.id,
      sessionId: taskB.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session B should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const cardA = testPage.getByTestId(`needs-you-item-${taskA.id}`);
    const cardB = testPage.getByTestId(`needs-you-item-${taskB.id}`);
    await expect(cardA).toBeVisible();
    await expect(cardB).toBeVisible();

    const boxA = await cardA.boundingBox();
    const boxB = await cardB.boundingBox();
    expect(boxA, "card A should have a box").not.toBeNull();
    expect(boxB, "card B should have a box").not.toBeNull();
    // One column: both cards share a left edge and stack vertically rather
    // than sitting side by side.
    expect(Math.abs(boxA!.x - boxB!.x)).toBeLessThanOrEqual(2);
    expect(boxB!.y).toBeGreaterThanOrEqual(boxA!.y + boxA!.height);

    const openTaskA = cardA.getByRole("link", { name: "Open task" });
    const openTaskBox = await openTaskA.boundingBox();
    expect(openTaskBox, "Open task action should have a box").not.toBeNull();
    expect(openTaskBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(openTaskBox!.width).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await expectNoHorizontalScroll(testPage);

    const strip = testPage.getByTestId("coordinator-count-strip");
    await expect(strip).toBeVisible();
    const stripPosition = await strip.evaluate(
      (el) => getComputedStyle(el.parentElement ?? el).position,
    );
    expect(stripPosition).toBe("sticky");
  });
});
