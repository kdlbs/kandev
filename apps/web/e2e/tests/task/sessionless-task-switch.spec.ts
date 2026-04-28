import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Bug: when a task is created via MCP `create_task_kandev` with
 * `start_agent=false`, it has no session/environment. Selecting that task in
 * the sidebar previously left the dockview layout corrupted (wrong widths on
 * the central group) and did not update the breadcrumb to the new task. The
 * corruption then persisted across subsequent task switches because the bad
 * layout was saved back to per-session storage.
 *
 * This test reproduces the failing flow end-to-end and asserts:
 *   1. Selecting the sessionless subtask updates the URL + breadcrumb.
 *   2. The dockview layout remains healthy (no zero-width groups).
 *   3. Switching back to the parent task keeps the layout healthy
 *      (regression on persisted-corruption).
 */

const SUBTASK_TITLE = "Sessionless MCP Subtask";

test.describe("Sessionless task switching", () => {
  test("selecting an MCP-created sessionless subtask keeps layout healthy", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Parent task agent script: create a subtask via MCP with start_agent=false.
    const script = [
      'e2e:thinking("Creating sessionless subtask...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task_kandev({"parent_id":"self","title":"${SUBTASK_TITLE}","description":"Sessionless subtask for layout-recovery test","start_agent":false})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sessionless Parent Task",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for parent agent to settle so the layout has stabilised.
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Poll the API until the MCP-created subtask exists. We need its ID for URL
    // assertions. The subtask is created by the agent asynchronously, so the
    // idle indicator alone isn't a guarantee.
    type TaskEntry = { id: string; title: string; primary_session_id?: string | null };
    let subtask: TaskEntry | undefined;
    await expect
      .poll(
        async () => {
          const all = await apiClient.listTasks(seedData.workspaceId);
          subtask = all.tasks.find((t: TaskEntry) => t.title === SUBTASK_TITLE);
          return subtask?.id ?? null;
        },
        { timeout: 30_000, message: "Waiting for MCP-created subtask to appear" },
      )
      .toBeTruthy();
    const subtaskId = subtask!.id;
    // Sanity: the bug requires a sessionless task. start_agent=false implies no
    // primary session was created on the backend.
    expect(subtask!.primary_session_id ?? null).toBeNull();

    // Baseline: layout is healthy on the parent.
    await session.expectLayoutHealthy();

    // Wait for the subtask to appear in the sidebar, then click it.
    const subtaskRow = session.taskInSidebar(SUBTASK_TITLE);
    await expect(subtaskRow).toBeVisible({ timeout: 10_000 });
    await session.clickTaskInSidebar(SUBTASK_TITLE);

    // URL should switch to the subtask within a reasonable time.
    await expect(testPage).toHaveURL(new RegExp(`/t/${subtaskId}(?:\\?|$)`), {
      timeout: 10_000,
    });

    // Breadcrumb (task title in the topbar) must reflect the new task.
    const breadcrumb = testPage.locator('[aria-current="page"]');
    await expect(breadcrumb).toHaveText(SUBTASK_TITLE, { timeout: 10_000 });

    // Layout must remain healthy on the sessionless task.
    await session.expectLayoutHealthy();

    // Switch back to the parent task. The bug surfaces here: persisted bad
    // layout state corrupts the central-group widths on this second switch.
    await session.clickTaskInSidebar("Sessionless Parent Task");
    await expect(testPage).toHaveURL(new RegExp(`/t/${parent.id}(?:\\?|$)`), {
      timeout: 10_000,
    });
    await expect(breadcrumb).toHaveText("Sessionless Parent Task", { timeout: 10_000 });
    await session.expectLayoutHealthy();
  });

  test("self-recovers when sessionStorage holds a corrupted layout for a known session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Two tasks with agents — A navigated first, then B, then back to A. The
    // back-to-A switch goes through performSessionSwitch's slow path which
    // historically applied the saved (potentially corrupt) blob verbatim.
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Layout Recovery Task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Layout Recovery Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await session.expectLayoutHealthy();

    // Resolve A's session id so we can target it precisely.
    type TaskEntry = { id: string; primary_session_id?: string | null };
    let sessionAId: string | null = null;
    await expect
      .poll(
        async () => {
          const all = await apiClient.listTasks(seedData.workspaceId);
          sessionAId =
            all.tasks.find((x: TaskEntry) => x.id === taskA.id)?.primary_session_id ?? null;
          return sessionAId;
        },
        { timeout: 15_000, message: "Waiting for task A session id" },
      )
      .toBeTruthy();

    // Switch to B, then back — this primes both layouts in sessionStorage.
    await session.clickTaskInSidebar("Layout Recovery Task B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}(?:\\?|$)`), { timeout: 10_000 });
    await session.expectLayoutHealthy();

    // Inject a corrupt layout for A while we're focused on B.
    await testPage.evaluate((sid) => {
      const corrupt = {
        grid: {
          root: {
            type: "leaf",
            size: 800,
            data: { id: "g1", views: ["chat"], activeView: "chat" },
          },
          height: 600,
          width: 0, // <-- corruption
          orientation: "HORIZONTAL",
        },
        panels: { chat: { id: "chat", contentComponent: "chat" } },
        activeGroup: "g1",
      };
      window.sessionStorage.setItem(`kandev.dockview.layout.${sid}`, JSON.stringify(corrupt));
    }, sessionAId!);

    // Switch back to A — performSessionSwitch should drop the corrupt blob
    // and rebuild the default instead of applying the zero-width layout.
    await session.clickTaskInSidebar("Layout Recovery Task A");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskA.id}(?:\\?|$)`), { timeout: 10_000 });
    await session.expectLayoutHealthy();
  });
});
