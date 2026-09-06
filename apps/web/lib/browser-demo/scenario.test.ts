/* eslint-disable max-lines-per-function */

import { describe, expect, it } from "vitest";
import {
  DEMO_IDS,
  createBootPayload,
  createDemoState,
  createTaskFromInput,
  demoPRFeedback,
} from "./scenario";
import { createDemoFiles } from "./demo-files";

describe("browser demo scenario", () => {
  it("hydrates a realistic board and GitHub pull request", () => {
    const state = createDemoState();
    const payload = createBootPayload(state);
    const snapshot = payload.initialState?.kanbanMulti?.snapshots[DEMO_IDS.workflow];

    expect(payload.initialState?.workspaces?.activeId).toBe(DEMO_IDS.workspace);
    expect(payload.initialState?.workflows?.items).toHaveLength(2);
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

  it("hydrates selectable task-creation profiles and runnable session environments", () => {
    const payload = createBootPayload(createDemoState());

    expect(payload.initialState?.agentProfiles?.items).toHaveLength(2);
    expect(payload.initialState?.executors?.items[0]?.profiles).toHaveLength(2);
    expect(payload.initialState?.availableAgents?.loaded).toBe(true);
    expect(payload.initialState?.workflows?.items[0]?.agent_profile_id).toBeUndefined();
    expect(payload.initialState?.environmentIdBySessionId?.["demo-session-audit"]).toBe(
      "demo-environment-demo-task-audit",
    );
  });

  it("shows structured agent capabilities in the checkout conversation", () => {
    const messages = createDemoState().messagesBySession["demo-session-checkout"];
    const types = new Set(messages.map((message) => message.type));

    expect(types).toEqual(
      new Set([
        "message",
        "thinking",
        "todo",
        "tool_search",
        "tool_read",
        "tool_execute",
        "tool_edit",
      ]),
    );
    expect(
      messages.find((message) => message.type === "tool_edit")?.metadata?.normalized,
    ).toMatchObject({
      modify_file: { mutations: [{ type: "patch" }] },
    });
    expect(
      new Set(messages.flatMap((message) => (message.turn_id ? [message.turn_id] : []))),
    ).toEqual(new Set(["checkout-turn", "checkout-followup"]));
    expect(messages).toHaveLength(13);
  });

  it("hydrates a multi-repository plan-mode task with a populated plan", () => {
    const state = createDemoState();
    const payload = createBootPayload(state);
    const task = state.tasks.find((entry) => entry.id === "demo-task-react");
    const session = state.sessions.find((entry) => entry.id === "demo-session-react");

    expect(task?.repositories).toMatchObject([
      { repository_id: DEMO_IDS.repository, base_branch: "main" },
      { repository_id: DEMO_IDS.apiRepository, base_branch: "develop" },
    ]);
    expect(session?.worktrees).toMatchObject([
      {
        repository_id: DEMO_IDS.repository,
        position: 0,
        worktree_path: "/demo/worktrees/demo-task-react/acme-web",
      },
      {
        repository_id: DEMO_IDS.apiRepository,
        position: 1,
        worktree_path: "/demo/worktrees/demo-task-react/acme-api",
      },
    ]);
    expect(
      state.messagesBySession["demo-session-react"].map((message) => message.type),
    ).not.toContain("agent_plan");
    expect(state.messagesBySession["demo-session-react"]).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ content: expect.stringContaining("acme-web/package.json") }),
        expect.objectContaining({ content: expect.stringContaining("acme-api/") }),
      ]),
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

  it("seeds believable multi-turn histories for active, review, and completed work", () => {
    const state = createDemoState();
    const audit = state.messagesBySession["demo-session-audit"];
    const auth = state.messagesBySession["demo-session-auth"];

    expect(audit.filter((message) => message.author_type === "user")).toHaveLength(2);
    expect(new Set(audit.map((message) => message.turn_id))).toEqual(
      new Set(["audit-implementation", "audit-review-fix"]),
    );
    expect(audit.map((message) => message.type)).toEqual(
      expect.arrayContaining(["thinking", "tool_search", "tool_edit", "tool_execute"]),
    );
    expect(auth.filter((message) => message.author_type === "user")).toHaveLength(2);
    expect(auth.at(-1)?.content).toContain("task is complete");
    expect(state.tasks.find((task) => task.id === "demo-task-auth")).toMatchObject({
      state: "COMPLETED",
      primary_session_id: "demo-session-auth",
    });
  });

  it("backs the audit approval badge with a pending chat permission", () => {
    const state = createDemoState();
    const task = state.tasks.find((candidate) => candidate.id === "demo-task-audit");
    const session = state.sessions.find((candidate) => candidate.id === "demo-session-audit");
    const messages = state.messagesBySession["demo-session-audit"];
    const permission = messages.find((message) => message.type === "permission_request");
    const toolCall = messages.find(
      (message) =>
        (message.metadata as { tool_call_id?: string } | undefined)?.tool_call_id ===
        "audit-migration-check",
    );

    expect(task).toMatchObject({
      primary_session_state: "WAITING_FOR_INPUT",
      primary_session_pending_action: "permission",
      review_status: "pending",
    });
    expect(session?.state).toBe("WAITING_FOR_INPUT");
    expect(toolCall).toMatchObject({ type: "tool_call", metadata: { status: "pending" } });
    expect(permission).toMatchObject({
      requests_input: true,
      metadata: {
        pending_id: "audit-migration-permission",
        tool_call_id: "audit-migration-check",
        action_type: "command",
        status: "pending",
      },
    });
  });

  it("provides a rich, navigable repository fixture", () => {
    const files = createDemoFiles();

    expect(Object.keys(files).length).toBeGreaterThanOrEqual(20);
    expect(files["src/checkout/complete-order.ts"]).toContain("withOrderLock");
    expect(files["src/audit/record-event.ts"]).toContain("coarseRegion");
    expect(files["migrations/20260718_create_audit_events.sql"]).toContain(
      "CREATE TABLE audit_events",
    );
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
