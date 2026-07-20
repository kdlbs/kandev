import { describe, expect, it } from "vitest";
import {
  DEMO_IDS,
  createBootPayload,
  createDemoState,
  createTaskFromInput,
  demoPRFeedback,
} from "./scenario";

describe("browser demo scenario", () => {
  it("hydrates a realistic board and GitHub pull request", () => {
    const state = createDemoState();
    const payload = createBootPayload(state);
    const snapshot = payload.initialState?.kanbanMulti?.snapshots[DEMO_IDS.workflow];

    expect(payload.initialState?.workspaces?.activeId).toBe(DEMO_IDS.workspace);
    expect(snapshot?.steps).toHaveLength(4);
    expect(snapshot?.tasks).toHaveLength(5);
    expect(payload.initialState?.repositories?.itemsByWorkspaceId[DEMO_IDS.workspace]).toHaveLength(
      2,
    );
    expect(payload.initialState?.taskPRs?.byTaskId["demo-task-audit"]?.[0]).toMatchObject({
      pr_number: 142,
      checks_state: "success",
      comment_count: 2,
    });
    expect(demoPRFeedback.comments).toHaveLength(2);
    expect(demoPRFeedback.comments[1].in_reply_to).toBe(demoPRFeedback.comments[0].id);
  });

  it("shows structured agent capabilities in the checkout conversation", () => {
    const messages = createDemoState().messagesBySession["demo-session-checkout"];
    const types = new Set(messages.map((message) => message.type));

    expect(types).toEqual(
      new Set(["message", "thinking", "tool_search", "tool_read", "tool_execute", "tool_edit"]),
    );
    expect(
      messages.find((message) => message.type === "tool_edit")?.metadata?.normalized,
    ).toMatchObject({
      modify_file: { mutations: [{ type: "patch" }] },
    });
    expect(messages.at(-1)?.content).toContain("```text");
  });

  it("hydrates a multi-repository plan-mode task with a populated plan", () => {
    const state = createDemoState();
    const payload = createBootPayload(state);
    const task = state.tasks.find((entry) => entry.id === "demo-task-react");

    expect(task?.repositories).toMatchObject([
      { repository_id: DEMO_IDS.repository, base_branch: "main" },
      { repository_id: DEMO_IDS.apiRepository, base_branch: "develop" },
    ]);
    expect(state.messagesBySession["demo-session-react"].map((message) => message.type)).toContain(
      "agent_plan",
    );
    expect(payload.initialState?.taskPlans?.byTaskId["demo-task-react"]?.content).toContain(
      "acme-api",
    );
    expect(payload.initialState?.chatInput?.planModeBySessionId["demo-session-react"]).toBe(true);
  });

  it("seeds a pending user question with selectable answers", () => {
    const question = createDemoState().messagesBySession["demo-session-empty"].find(
      (message) => message.type === "clarification_request",
    );

    expect(question).toMatchObject({
      requests_input: true,
      metadata: {
        status: "pending",
        question: {
          id: "empty-state-action",
          options: [{ option_id: "connect" }, { option_id: "invite" }],
        },
      },
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
