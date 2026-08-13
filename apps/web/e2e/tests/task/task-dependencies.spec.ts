import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

// Task dependencies: a peer "B cannot start until A finishes" relationship,
// separate from the parent/child subtask hierarchy. These specs cover the
// guarantees that only hold end to end, where the create path, the event bus,
// the auto-start chokepoint, and the derived projection all participate:
//
//   - a blocked task is not launched by any automated path,
//   - only SUCCESS resolves a dependency, so a failed predecessor halts a chain,
//   - resolution launches the dependent exactly once and never restarts the
//     predecessor,
//   - a cycle is refused with the offending path,
//   - the projection reaches the browser, which is where it kept getting lost.
//
// Predecessors are seeded WITHOUT an agent and completed over the API. That is
// deliberate: driving the terminal transition directly exercises the same
// task.state_changed path a real agent turn ends on, without making the assertion
// depend on mock-agent timing. The DEPENDENTS carry agent profiles, because
// whether they launch is the thing under test.

/** Sessions a task owns. A blocked task must have none. */
async function sessionCount(apiClient: ApiClient, taskId: string): Promise<number> {
  const { total } = await apiClient.listTaskSessions(taskId);
  return total;
}

/** Poll until a task owns at least one session, failing with a readable message. */
async function expectStarted(apiClient: ApiClient, taskId: string, label: string): Promise<void> {
  await expect
    .poll(() => sessionCount(apiClient, taskId), {
      timeout: 30_000,
      message: `${label} should have been launched by dependency resolution`,
    })
    .toBeGreaterThan(0);
}

/**
 * Assert a task stays sessionless. Dependency resolution is asynchronous, so a
 * single read would pass before the launch it is meant to catch. Poll for a
 * fixed window and require the count to stay at zero throughout.
 */
async function expectStaysUnstarted(
  apiClient: ApiClient,
  taskId: string,
  label: string,
): Promise<void> {
  const deadline = Date.now() + 6_000;
  while (Date.now() < deadline) {
    expect(await sessionCount(apiClient, taskId), `${label} must not be started`).toBe(0);
    // Polling interval for the dwell described above, not a settle: the
    // assertion is that nothing starts during the window, so the loop has to
    // keep sampling across it rather than wait for any single event.
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
}

test.describe("Task dependencies", () => {
  test("a chain runs one step at a time and never restarts a predecessor", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const a = await apiClient.createTask(seedData.workspaceId, "Chain A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const withAgent = {
      description: 'e2e:message("step done")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const b = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Chain B",
      seedData.agentProfileId,
      { ...withAgent, blocked_by: [a.id] },
    );
    const c = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Chain C",
      seedData.agentProfileId,
      { ...withAgent, blocked_by: [b.id] },
    );

    // A create that declares dependencies records its start rather than firing
    // it, even though start_agent defaulted to true.
    expect(b.session_id, "B must not be launched on create").toBeFalsy();
    expect(c.session_id, "C must not be launched on create").toBeFalsy();
    expect(await sessionCount(apiClient, b.id)).toBe(0);
    expect(await sessionCount(apiClient, c.id)).toBe(0);

    const blockedB = await apiClient.getTaskDependencies(b.id);
    expect(blockedB.blocked).toBe(true);
    expect(blockedB.blocked_reason).toBe("pending");
    expect(blockedB.depends_on?.map((d) => d.id)).toEqual([a.id]);
    // The reverse direction is derived from the same edge, not stored twice.
    expect((await apiClient.getTaskDependencies(a.id)).blocks?.map((d) => d.id)).toEqual([b.id]);

    await apiClient.updateTaskState(a.id, "COMPLETED");

    await expectStarted(apiClient, b.id, "B");
    // C is two hops out. Resolution must not walk the graph transitively.
    await expectStaysUnstarted(apiClient, c.id, "C while B is still running");

    const bSessions = await sessionCount(apiClient, b.id);
    await apiClient.updateTaskState(b.id, "COMPLETED");

    await expectStarted(apiClient, c.id, "C");
    // The predecessors are untouched by their dependents starting. A restarted
    // A is the failure this chain exists to prevent.
    expect(await sessionCount(apiClient, a.id), "A must not be restarted").toBe(0);
    expect(await sessionCount(apiClient, b.id), "B must launch exactly once").toBe(bSessions);
  });

  test("a failed predecessor halts the chain until it succeeds", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const predecessor = await apiClient.createTask(seedData.workspaceId, "Halt A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const dependent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Halt B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("should only run after a success")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        blocked_by: [predecessor.id],
      },
    );

    await apiClient.updateTaskState(predecessor.id, "FAILED");

    // FAILED is terminal for the parent/child completion signal but NOT for a
    // dependency: the chain stops and waits for a human.
    await expectStaysUnstarted(apiClient, dependent.id, "the dependent of a failed predecessor");
    const blocked = await apiClient.getTaskDependencies(dependent.id);
    expect(blocked.blocked).toBe(true);
    expect(blocked.blocked_reason).toBe("failed");
    expect(blocked.depends_on?.[0]?.status).toBe("failed");

    // Retrying the predecessor to success is one of the three ways out, and the
    // intent must survive the failed attempt rather than having been consumed.
    await apiClient.updateTaskState(predecessor.id, "COMPLETED");
    await expectStarted(apiClient, dependent.id, "the dependent after a retry");
  });

  test("removing the last edge unblocks without launching", async ({ apiClient, seedData }) => {
    const predecessor = await apiClient.createTask(seedData.workspaceId, "Edge A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const dependent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Edge B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("unreachable")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        blocked_by: [predecessor.id],
      },
    );

    const after = await apiClient.removeTaskDependency(dependent.id, predecessor.id);
    expect(after.blocked).toBe(false);
    expect(after.depends_on ?? []).toEqual([]);

    // Unblocking by edge removal is not success, so the recorded intent must
    // not fire. Only a resolved predecessor starts a dependent.
    await expectStaysUnstarted(apiClient, dependent.id, "a dependent unblocked by edge removal");

    // Removing an edge that is not there is a success no-op, not a 404.
    await expect(
      apiClient.removeTaskDependency(dependent.id, predecessor.id),
    ).resolves.toBeTruthy();
  });

  test("a cycle is refused with the offending path", async ({ apiClient, seedData }) => {
    const base = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const a = await apiClient.createTask(seedData.workspaceId, "Cycle A", base);
    const b = await apiClient.createTask(seedData.workspaceId, "Cycle B", base);
    const c = await apiClient.createTask(seedData.workspaceId, "Cycle C", base);

    await apiClient.addTaskDependency(b.id, a.id);
    await apiClient.addTaskDependency(c.id, b.id);

    // A depending on C would close A -> C -> B -> A.
    const response = await apiClient.rawAddTaskDependency(a.id, c.id);
    expect(response.status).toBe(409);
    const body = (await response.json()) as { cycle?: string[] };
    expect(body.cycle, "the 409 must carry the path so the UI can render it").toEqual([
      a.id,
      c.id,
      b.id,
      a.id,
    ]);

    // The rejected edge must not have been written.
    expect((await apiClient.getTaskDependencies(a.id)).depends_on ?? []).toEqual([]);
  });

  test("the board and the open task both show the dependency", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const predecessor = await apiClient.createTask(seedData.workspaceId, "Visible A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const dependent = await apiClient.createTask(seedData.workspaceId, "Visible B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      blocked_by: [predecessor.id],
    });

    await testPage.goto("/");
    await expect(
      testPage.getByTestId(`task-card-${dependent.id}`).getByTestId("kanban-card-blocked-badge"),
    ).toBeVisible();
    await expect(
      testPage.getByTestId(`task-card-${predecessor.id}`).getByTestId("kanban-card-blocked-badge"),
    ).toHaveCount(0);

    // The chip has to survive the boot payload, the WS event stream, and the
    // store projection. Every regression in this feature so far has been a
    // dropped hop between those, so assert it from the rendered page.
    await testPage.goto(`/t/${dependent.id}`);
    const chip = testPage.getByTestId("task-dependency-chip");
    await expect(chip).toBeVisible();
    await chip.click();
    await expect(
      testPage.getByTestId("task-dependency-entry").filter({ hasText: "Visible A" }),
    ).toBeVisible();

    // A task with no edges in either direction renders no chip at all.
    const unrelated = await apiClient.createTask(seedData.workspaceId, "Visible C", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await testPage.goto(`/t/${unrelated.id}`);
    await expect(testPage.getByTestId("task-dependency-chip")).toHaveCount(0);
  });
});
