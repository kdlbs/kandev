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
import { linkToCoordinator, linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

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
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
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

  // The phone nav's Inbox/Coordinator rows live in the "saved sidebar layout"
  // navigation path (MobileRequiredRows), which only renders once the
  // workspace has a customized layout on record (revision > 0); a workspace
  // that has never been customized falls back to the legacy static
  // destination list, which has no Inbox/Coordinator entries at all. A
  // freshly created workspace starts at revision 0, so this seeds a minimal
  // layout first. The phone nav also reads the active-workspace store slice,
  // not the coordinator route's URL param (the coordinator route never syncs
  // the two, unlike kanban/office routes), so the workspace is switched to
  // through the real picker (mobile-workspace-trigger/-item, the
  // app-nav-sheet's own copy of the sidebar's workspace switcher) before
  // checking it. seedData.workspaceId is not usable for either concern: it
  // is a worker-scoped fixture shared across every test in the run, so its
  // layout/coordinator state depends on what earlier specs left behind.
  test("with no coordinator the phone nav shows the Coordinators section with the set-up row and no strip (task-04 Acceptance)", async ({
    testPage,
    apiClient,
  }) => {
    const workspace = await apiClient.createWorkspace(`Coordinator Mobile Empty ${Date.now()}`);
    try {
      await apiClient.saveUserSettings({
        sidebar_layout_state: {
          workspace_id: workspace.id,
          expected_revision: 0,
          layout: {
            version: 1,
            revision: 0,
            nodes: [{ id: "home", kind: "builtin", visible: true, destination_id: "home" }],
          },
        },
      });

      await testPage.goto("/");
      await testPage.getByTestId("app-nav-trigger").tap();
      const switchMenu = testPage.getByTestId("app-nav-sheet");
      await expect(switchMenu).toBeVisible();
      await switchMenu.getByTestId("mobile-workspace-trigger").tap();
      // The dropdown's own menu content renders in a portal outside the
      // app-nav-sheet dialog, so the item lookup is unscoped (matching the
      // desktop picker's sidebar-workspace-item-* pattern).
      await testPage.getByTestId(`mobile-workspace-item-${workspace.id}`).tap();

      await testPage.goto(linkToCoordinator(workspace.id));
      await expect(testPage.getByTestId("no-coordinator-state")).toBeVisible();
      await expect(testPage.getByTestId("coordinator-count-strip")).toHaveCount(0);

      await testPage.getByTestId("app-nav-trigger").tap();
      const menu = testPage.getByTestId("app-nav-sheet");
      await expect(menu).toBeVisible();

      const section = menu.getByTestId("mobile-coordinators-section");
      await expect(section).toBeVisible();
      await expect(section.getByRole("button", { name: /Coordinators/ })).toHaveAttribute(
        "aria-expanded",
        "true",
      );
      const emptyRow = menu.getByTestId("mobile-sidebar-coordinators-empty");
      await expect(emptyRow).toBeVisible();
      await expect(emptyRow).toHaveText("Set up a coordinator");
    } finally {
      await apiClient.deleteWorkspace(workspace.id, workspace.name);
    }
  });

  // The legacy nav path (no saved sidebar layout) is the default for every
  // user who never customised the sidebar, so it gets its own case.
  test("the default phone menu, with no saved sidebar layout, shows the Coordinators section above Automations (AC-COORDINATOR-NEEDS-YOU-006.1)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workspace = await apiClient.createWorkspace(`Coordinator Mobile Default ${Date.now()}`);
    try {
      const coordinator = await apiClient.createCoordinator(workspace.id, {
        name: "Default Layout Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });

      await testPage.goto("/");
      await testPage.getByTestId("app-nav-trigger").tap();
      const picker = testPage.getByTestId("app-nav-sheet");
      await expect(picker).toBeVisible();
      await picker.getByTestId("mobile-workspace-trigger").tap();
      await testPage.getByTestId(`mobile-workspace-item-${workspace.id}`).tap();

      await testPage.getByTestId("app-nav-trigger").tap();
      const menu = testPage.getByTestId("app-nav-sheet");
      await expect(menu).toBeVisible();
      const section = menu.getByTestId("mobile-coordinators-section");
      await expect(section).toBeVisible();
      await expect(
        section.getByTestId(`mobile-sidebar-coordinator-${coordinator.id}`),
      ).toBeVisible();
      const sectionBox = await section.boundingBox();
      const automationsBox = await menu.getByTestId("mobile-automations-section").boundingBox();
      expect(sectionBox).not.toBeNull();
      expect(automationsBox).not.toBeNull();
      expect(sectionBox!.y).toBeLessThan(automationsBox!.y);
    } finally {
      await apiClient.deleteWorkspace(workspace.id, workspace.name);
    }
  });
});
