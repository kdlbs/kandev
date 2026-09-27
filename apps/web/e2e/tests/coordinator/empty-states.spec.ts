// AC-COORDINATOR-NEEDS-YOU-006.4, -007.1..007.5: the coordinator route's
// missing/empty/failure states (docs/specs/coordinator/requirements/needs-you.md,
// #routes-and-sidebar, #failure-and-recovery).
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinator, linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

test.describe("Coordinator missing/empty/failure states", () => {
  test("a workspace with no coordinator shows the no-coordinator state (AC .006.4, .007.2)", async ({
    testPage,
    apiClient,
  }) => {
    const workspace = await apiClient.createWorkspace(`Coordinator Empty ${Date.now()}`);
    try {
      await testPage.goto(linkToCoordinator(workspace.id));
      await expect(testPage.getByTestId("no-coordinator-state")).toBeVisible();
      await expect(testPage.getByTestId("coordinator-count-strip")).toHaveCount(0);
    } finally {
      await apiClient.deleteWorkspace(workspace.id, workspace.name);
    }
  });

  // Sidebar rows read the active-workspace store slice, not the coordinator
  // route's URL param (the coordinator route never syncs the two, unlike
  // kanban/office routes), so a freshly created workspace only becomes what
  // the sidebar renders once switched to through the real picker. seedData.workspaceId
  // is not usable here either: it is a worker-scoped fixture shared across
  // every test in the run, and other specs create coordinators on it, so it
  // is not reliably coordinator-free by the time this test executes.
  test("with no coordinator the sidebar Inbox row is unchanged and the generic entry has no badge (task-04 Acceptance)", async ({
    testPage,
    apiClient,
  }) => {
    const workspace = await apiClient.createWorkspace(`Coordinator Empty Sidebar ${Date.now()}`);
    try {
      await testPage.goto("/");
      await testPage.getByTestId("sidebar-workspace-trigger").click();
      await testPage.getByTestId(`sidebar-workspace-item-${workspace.id}`).click();

      await testPage.goto(linkToCoordinator(workspace.id));
      await expect(testPage.getByTestId("no-coordinator-state")).toBeVisible();
      await expect(testPage.getByTestId("coordinator-count-strip")).toHaveCount(0);

      await expect(testPage.getByTestId("sidebar-needs-you-inbox")).toBeVisible();
      const genericRow = testPage.getByTestId("sidebar-coordinator-generic");
      await expect(genericRow).toBeVisible();
      await expect(genericRow).toHaveText("Coordinator");
    } finally {
      await apiClient.deleteWorkspace(workspace.id, workspace.name);
    }
  });

  test("an id not in this workspace shows the unknown-coordinator state, not a 404 (AC .006.4)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Real Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(
      linkToCoordinatorNeedsYou(seedData.workspaceId, "not-a-real-coordinator-id"),
    );

    await expect(testPage.getByTestId("unknown-coordinator-state")).toBeVisible();
    await expect(testPage.getByTestId("coordinator-count-strip")).toHaveCount(0);
    await expect(testPage.getByRole("link", { name: "See coordinators" })).toBeVisible();
  });

  test("an empty Needs you list reads as the working state, not a dead end (AC .007.1)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Empty State Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const empty = testPage.getByTestId("empty-needs-you-state");
    await expect(empty).toBeVisible();
    await expect(empty).toContainText("Nothing needs you. That is the working state.");
    await expect(empty).toContainText("0 cards are running.");
    await expect(empty.getByRole("link", { name: "See what is running" })).toBeVisible();
  });

  test("a failed stalls read shows the per-input banner and recovers on Try again (AC .007.3, .007.5)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Banner Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    let stallsCallCount = 0;
    const stallsUrl = `${backend.baseUrl}/api/v1/workspaces/${seedData.workspaceId}/coordinator-stalls`;
    await testPage.route(stallsUrl, async (route) => {
      stallsCallCount += 1;
      if (stallsCallCount === 1) {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "simulated stalls failure" }),
        });
        return;
      }
      await route.continue();
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const banner = testPage.getByTestId("coordinator-input-failure-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("Could not load stall records.");
    // Only the stalls input failed: the rest of the screen still renders
    // (AC .007.3's banner never replaces the list wholesale).
    await expect(testPage.getByTestId("coordinator-count-strip")).toBeVisible();

    await banner.getByRole("button", { name: "Try again" }).click();
    await expect(banner).toHaveCount(0, { timeout: 15_000 });
    expect(stallsCallCount).toBeGreaterThanOrEqual(2);
  });
});
