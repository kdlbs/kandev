// AC-UI-INBOX-FAILED-001.29: observing the Failed tab's count before that tab
// is ever selected, then selecting it, observing a failed task listed with
// its reason and relative failure time, opening the task from the row, and
// observing that the sidebar count did not change when the failed task
// appeared.
import { test, expect } from "../../fixtures/test-base";
import { injectLatency, waitForHttp } from "../../helpers/causal-waits";

test.describe("Inbox Failed tab", () => {
  test("lists a failed task without inflating the sidebar count (AC .29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const title = "Inbox Failed Tab Fixture";
    const reason = "Simulated failure for the Inbox Failed tab";

    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTaskSession(taskId, {
      state: "FAILED",
      completedAt: new Date().toISOString(),
      errorMessage: reason,
    });
    await apiClient.updateTaskState(taskId, "FAILED");

    await testPage.goto("/needs-you-inbox");

    // The sidebar's "someone is blocked on you" count must never move when a
    // failed task exists (AC .14) -- captured before the Failed tab is ever
    // selected, so an accidental read into the shared count would show here.
    const sidebarInbox = testPage.getByTestId("sidebar-needs-you-inbox");
    await expect(sidebarInbox).toBeVisible();
    const sidebarTextBeforeFailedTabSelected = await sidebarInbox.textContent();

    // The tab strip's own Failed count becomes visible on Inbox mount --
    // before the Failed tab is ever the selected one (AC .29's ordering).
    const failedBadge = testPage.getByTestId("inbox-tab-failed-badge");
    await expect(failedBadge).toHaveText("1", { timeout: 30_000 });

    await testPage.getByRole("tab", { name: /Failed/ }).click();

    const row = testPage.getByTestId("failed-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText(reason);

    // A real failure instant renders a relative time, not the "unknown" fallback.
    await expect(row.getByTestId("failed-inbox-unknown-time")).toHaveCount(0);

    await row.getByTestId("failed-inbox-open-task").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}$`));

    const sidebarTextAfterOpeningTask = await sidebarInbox.textContent();
    expect(sidebarTextAfterOpeningTask).toBe(sidebarTextBeforeFailedTabSelected);
  });

  // AC-UI-INBOX-FAILED-001.16: "Changing the active workspace shall clear it
  // until a response for the new one is applied." Regression coverage for the
  // Review-round-2 blocker: returning to a workspace visited earlier in the
  // same session must not keep showing that workspace's stale ready badge
  // while the fresh read for it is still in flight.
  test("clears the stale badge while re-reading a re-visited workspace (AC .16)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const title = "Workspace Switch Back Fixture";
    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTaskSession(taskId, {
      state: "FAILED",
      completedAt: new Date().toISOString(),
      errorMessage: "Simulated failure for the workspace-switch regression",
    });
    await apiClient.updateTaskState(taskId, "FAILED");

    const workspaceB = await apiClient.createWorkspace("Failed Tab Switch Target");

    const failedBadge = testPage.getByTestId("inbox-tab-failed-badge");

    // First visit to workspace A's Failed tab: reads to "ready" with 1 row,
    // the cached state this test later returns to.
    await testPage.goto("/needs-you-inbox");
    await expect(failedBadge).toHaveText("1", { timeout: 30_000 });

    // Switch to workspace B (client-side nav; unmounts the Inbox page and its
    // controller) and visit its own empty Failed tab, so the slice's
    // last-active-workspace tracking actually flips away from A.
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${workspaceB.id}`).click();
    await expect(testPage).toHaveURL((url) => url.pathname === "/");
    await testPage.getByTestId("sidebar-needs-you-inbox").click();
    await expect(testPage).toHaveURL(/\/needs-you-inbox$/);
    // Anchor the absence check to a point after the tab strip has actually
    // rendered -- checking `toHaveCount(0)` immediately after the URL change
    // could pass trivially during the pre-mount window and never re-poll
    // once satisfied, so it would not reliably catch a badge that renders
    // (correctly or, if regressed, stale) a few milliseconds later.
    await expect(testPage.getByRole("tab", { name: /Failed/ })).toBeVisible();
    await expect(failedBadge).toHaveCount(0);

    // Switch back to workspace A (still client-side; the store is never torn
    // down) and delay only the next failed-inbox read, so the in-flight
    // "loading" window is observable instead of resolving instantly.
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${seedData.workspaceId}`).click();
    await expect(testPage).toHaveURL((url) => url.pathname === "/");

    let delayNextFailedInboxRead = false;
    await testPage.route("**/api/v1/failed-inbox*", async (route) => {
      if (delayNextFailedInboxRead) {
        delayNextFailedInboxRead = false;
        await injectLatency(
          1500,
          "make the re-visited workspace's loading window observable before it resolves",
        );
      }
      await route.continue();
    });
    delayNextFailedInboxRead = true;
    const rereadForA = waitForHttp(testPage, "GET", /\/failed-inbox$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === seedData.workspaceId,
    });

    await testPage.getByTestId("sidebar-needs-you-inbox").click();
    await expect(testPage).toHaveURL(/\/needs-you-inbox$/);
    // Anchored the same way as the switch-to-B assertion above.
    await expect(testPage.getByRole("tab", { name: /Failed/ })).toBeVisible();

    // The bug this covers: workspace A's own cache was still "ready" from the
    // first visit, so the badge kept showing its stale "1" immediately
    // instead of clearing while this re-read is in flight.
    await expect(failedBadge).toHaveCount(0);

    await rereadForA;
    await expect(failedBadge).toHaveText("1", { timeout: 15_000 });
  });

  // Regression coverage for the Review-round-3 blocker: the sibling test
  // above visits workspace B's own Failed tab before switching back, which is
  // exactly what let the stale cache go undetected -- `beginFailedInboxRead`
  // is only ever called while the Inbox is mounted, so a switch that happens
  // with the Inbox closed throughout must still be caught the next time this
  // workspace is read.
  test("clears a re-visited workspace's stale badge even when the switch away happened with the Inbox closed (AC .16)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const title = "Workspace Switch Back While Closed Fixture";
    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTaskSession(taskId, {
      state: "FAILED",
      completedAt: new Date().toISOString(),
      errorMessage: "Simulated failure for the closed-Inbox workspace-switch regression",
    });
    await apiClient.updateTaskState(taskId, "FAILED");

    const workspaceB = await apiClient.createWorkspace("Failed Tab Switch Target (Closed)");

    const failedBadge = testPage.getByTestId("inbox-tab-failed-badge");

    // First visit to workspace A's Failed tab: reads to "ready" with 1 row,
    // the cached state this test later returns to.
    await testPage.goto("/needs-you-inbox");
    await expect(failedBadge).toHaveText("1", { timeout: 30_000 });

    // Switch to workspace B but never open its Inbox -- stay on the landing
    // page. Nothing here calls the failed-inbox read for B at all, unlike
    // the sibling test above. The switcher is a controlled Radix dropdown
    // (app-sidebar-header.tsx wires its open state through the store) whose
    // content has a CSS exit animation (dropdown-menu.tsx's
    // `data-closed:animate-out ... duration-100`); Radix keeps the departing
    // item mounted until that animation ends, after `aria-expanded` has
    // already flipped. Both switches land on pathname "/" (only the
    // `workspaceId` query param changes), so the routed page never remounts
    // to force a settle the way every other picker test's route change does
    // -- wait for the departing item to actually leave the DOM before
    // reopening the same trigger, or the second open can race Radix's
    // in-flight close and land unstable.
    const workspaceTrigger = testPage.getByTestId("sidebar-workspace-trigger");
    const itemB = testPage.getByTestId(`sidebar-workspace-item-${workspaceB.id}`);
    await workspaceTrigger.click();
    await itemB.click();
    await expect(testPage).toHaveURL((url) => url.pathname === "/");
    await expect(itemB).toHaveCount(0);

    // Switch back to workspace A, still without ever having opened the Inbox
    // on B.
    const itemA = testPage.getByTestId(`sidebar-workspace-item-${seedData.workspaceId}`);
    await workspaceTrigger.click();
    await itemA.click();
    await expect(testPage).toHaveURL((url) => url.pathname === "/");
    await expect(itemA).toHaveCount(0);
    await expect(workspaceTrigger).toHaveAttribute("aria-expanded", "false");

    let delayNextFailedInboxRead = false;
    await testPage.route("**/api/v1/failed-inbox*", async (route) => {
      if (delayNextFailedInboxRead) {
        delayNextFailedInboxRead = false;
        await injectLatency(
          1500,
          "make the re-visited workspace's loading window observable before it resolves",
        );
      }
      await route.continue();
    });
    delayNextFailedInboxRead = true;
    const rereadForA = waitForHttp(testPage, "GET", /\/failed-inbox$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("workspace_id") === seedData.workspaceId,
    });

    await testPage.getByTestId("sidebar-needs-you-inbox").click();
    await expect(testPage).toHaveURL(/\/needs-you-inbox$/);
    await expect(testPage.getByRole("tab", { name: /Failed/ })).toBeVisible();

    // The bug this covers: because the Inbox was never open while B was
    // active, the slice's workspace-change tracking never saw the switch,
    // so A's cache was still "ready" from the first visit and the badge
    // kept showing its stale "1" instead of clearing while this re-read is
    // in flight.
    await expect(failedBadge).toHaveCount(0);

    await rereadForA;
    await expect(failedBadge).toHaveText("1", { timeout: 15_000 });
  });
});
