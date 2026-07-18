import { describe, expect, it } from "vitest";
import { DEMO_IDS, createBootPayload, createDemoState, createTaskFromInput } from "./scenario";

describe("browser demo scenario", () => {
  it("hydrates a realistic board and GitHub pull request", () => {
    const state = createDemoState();
    const payload = createBootPayload(state);
    const snapshot = payload.initialState?.kanbanMulti?.snapshots[DEMO_IDS.workflow];

    expect(payload.initialState?.workspaces?.activeId).toBe(DEMO_IDS.workspace);
    expect(snapshot?.steps).toHaveLength(4);
    expect(snapshot?.tasks).toHaveLength(5);
    expect(payload.initialState?.taskPRs?.byTaskId["demo-task-audit"]?.[0]).toMatchObject({
      pr_number: 142,
      checks_state: "success",
    });
  });

  it("creates task DTOs compatible with the normal kanban API", () => {
    const state = createDemoState();
    const task = createTaskFromInput(state, {
      title: "Demo-created task",
      description: "Created without a server",
      start_agent: true,
    });

    expect(task).toMatchObject({
      title: "Demo-created task",
      state: "IN_PROGRESS",
      workflow_id: DEMO_IDS.workflow,
      workflow_step_id: DEMO_IDS.steps.backlog,
    });
    expect(task.repositories?.[0]?.repository_id).toBe(DEMO_IDS.repository);
  });
});
