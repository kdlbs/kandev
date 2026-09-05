import { describe, expect, it } from "vitest";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { ThreadFilterClause, ThreadView } from "@/lib/state/slices/ui/thread-view-types";
import { queryThreadView, selectThreadCandidates } from "./thread-view-query";

type TaskOverrides = Partial<KanbanState["tasks"][number]> & { id: string };
const REVIEW_STEP_ID = "step-review";
const SECOND_WORKFLOW_ID = "workflow-2";
const SAME_ACTIVITY = "2026-08-31T12:00:00Z";

function task(overrides: TaskOverrides): KanbanState["tasks"][number] {
  const { id, ...rest } = overrides;
  return {
    id,
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    workflowStepId: "step-build",
    title: `Task ${id}`,
    position: 0,
    state: "IN_PROGRESS",
    primarySessionId: `session-${id}`,
    primarySessionState: "RUNNING",
    updatedAt: "2026-08-31T10:00:00Z",
    createdAt: "2026-08-30T10:00:00Z",
    ...rest,
  };
}

function summary(overrides: Partial<TaskStatusSummary> = {}): TaskStatusSummary {
  return {
    revision: 1,
    updated_at: "2026-08-31T10:00:00Z",
    primary_session: { id: "session-default", state: "RUNNING" },
    ...overrides,
  };
}

function snapshot(
  tasks: KanbanState["tasks"],
  workflowId = "workflow-1",
  workflowName = "Delivery",
): WorkflowSnapshotData {
  return {
    workflowId,
    workflowName,
    steps: [
      { id: REVIEW_STEP_ID, title: "Review", color: "green", position: 1 },
      { id: "step-build", title: "Build", color: "blue", position: 0 },
    ],
    tasks,
  };
}

function view(overrides: Partial<ThreadView> = {}): ThreadView {
  return {
    id: "view-all-threads",
    name: "All threads",
    taskScope: { mode: "all", taskIds: [] },
    filters: [],
    sort: { key: "attention", direction: "asc" },
    maxColumns: null,
    ...overrides,
  };
}

function clause(
  dimension: ThreadFilterClause["dimension"],
  value: ThreadFilterClause["value"],
  op: ThreadFilterClause["op"] = "is",
): ThreadFilterClause {
  return { id: `${dimension}-${op}`, dimension, op, value };
}

function richTask(): KanbanState["tasks"][number] {
  return task({
    id: "rich",
    workflowStepId: REVIEW_STEP_ID,
    title: "Fix checkout regression",
    priority: "critical",
    state: "REVIEW",
    blocked: true,
    labels: ["urgent", "frontend"],
    primaryAgentProfileId: "agent-profile-1",
    primaryAgentName: "Reviewer",
    primaryExecutorType: "ssh",
    foregroundActivity: "background",
    interrupted: true,
    sessionCount: 2,
    isPRReview: true,
    statusSummary: summary({
      primary_session: { id: "session-rich", state: "WAITING_FOR_INPUT" },
      pending_action: "permission",
      queued_prompt_count: 2,
      active_subagent_count: 1,
      active_error: {
        stamp: "error-1",
        occurred_at: "2026-08-31T09:00:00Z",
        preview: "The check failed",
      },
      git: { changed_files: 2 },
      foreground_activity: "background",
      pull_request: {
        count: 1,
        open_count: 1,
        attention: true,
        number: 42,
        state: "open",
        aggregate_state: "checks_failed",
      },
      last_activity_at: "2026-08-31T11:00:00Z",
    }),
  });
}

describe("selectThreadCandidates", () => {
  it("projects bounded task data without applying the active workflow filter", () => {
    const candidates = selectThreadCandidates({
      "workflow-1": snapshot([richTask()]),
      [SECOND_WORKFLOW_ID]: snapshot(
        [
          task({
            id: "other-workflow",
            workflowId: SECOND_WORKFLOW_ID,
            workspaceId: "workspace-1",
          }),
        ],
        SECOND_WORKFLOW_ID,
        "Operations",
      ),
    });

    expect(candidates.map((candidate) => candidate.taskId)).toEqual(
      expect.arrayContaining(["rich", "other-workflow"]),
    );
    expect(candidates).toHaveLength(2);
    expect(candidates.find((candidate) => candidate.taskId === "rich")).toMatchObject({
      workflowStepId: REVIEW_STEP_ID,
      taskState: "REVIEW",
      priority: "critical",
      blocked: true,
      labels: ["urgent", "frontend"],
      taskType: "pull_request_review",
      primaryAgentProfileId: "agent-profile-1",
      executorType: "ssh",
      repositoryIds: [],
      threadStatus: "needs_action",
      hasDiff: true,
      hasPR: true,
      prNeedsAttention: true,
      hasActiveError: true,
      hasMultipleSessions: true,
      foregroundActivity: "background",
      interrupted: true,
      isOnLastWorkflowStep: true,
      prInfo: {
        number: 42,
        state: "Open",
        aggregateState: "checks_failed",
      },
    });
  });

  it("keeps selected IDs out of the result while they are ineligible", () => {
    const result = queryThreadView(
      {
        "workflow-1": snapshot([
          task({ id: "selected", primarySessionState: "COMPLETED" }),
          task({ id: "running" }),
        ]),
      },
      view({ taskScope: { mode: "selected", taskIds: ["selected", "running"] } }),
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["running"]);
    expect(result.candidates.map((candidate) => candidate.taskId)).toEqual(["running"]);
  });
});

describe("queryThreadView filters", () => {
  const filterCases: Array<{
    dimension: ThreadFilterClause["dimension"];
    value: ThreadFilterClause["value"];
    op?: ThreadFilterClause["op"];
  }> = [
    { dimension: "threadStatus", value: "needs_action" },
    { dimension: "pendingAction", value: "permission" },
    { dimension: "taskState", value: "REVIEW" },
    { dimension: "workflow", value: "workflow-1" },
    { dimension: "workflowStep", value: REVIEW_STEP_ID },
    { dimension: "repository", value: "repo-1" },
    { dimension: "primaryAgent", value: "agent-profile-1" },
    { dimension: "executorType", value: "ssh" },
    { dimension: "priority", value: "critical" },
    { dimension: "blocked", value: true },
    { dimension: "hasQueuedPrompts", value: true },
    { dimension: "hasActiveSubagents", value: true },
    { dimension: "hasDiff", value: true },
    { dimension: "hasPR", value: true },
    { dimension: "prNeedsAttention", value: true },
    { dimension: "taskType", value: "pull_request_review" },
    { dimension: "titleMatch", value: "CHECKOUT", op: "matches" },
    { dimension: "hasActiveError", value: true },
    { dimension: "taskLabel", value: "urgent" },
    { dimension: "taskOrigin", value: "automation_run" },
    { dimension: "hasMultipleSessions", value: true },
  ];

  it.each(filterCases)("supports the $dimension dimension", ({ dimension, value, op }) => {
    const candidateTask = richTask();
    candidateTask.repositories = [
      {
        id: "task-repo-1",
        repository_id: "repo-1",
        base_branch: "main",
        position: 0,
      },
    ];
    candidateTask.metadata = { origin: "automation_run" };
    const snapshots: Record<string, WorkflowSnapshotData> =
      dimension === "workflow"
        ? {
            "workflow-1": snapshot([candidateTask]),
            [SECOND_WORKFLOW_ID]: snapshot(
              [task({ id: "plain", workflowId: SECOND_WORKFLOW_ID })],
              SECOND_WORKFLOW_ID,
              "Operations",
            ),
          }
        : { "workflow-1": snapshot([candidateTask, task({ id: "plain" })]) };
    const result = queryThreadView(
      snapshots,
      view({ filters: [clause(dimension, value, op ?? "is")] }),
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["rich"]);
  });

  it("uses AND between clauses and OR inside an in clause", () => {
    const result = queryThreadView(
      {
        "workflow-1": snapshot([
          richTask(),
          task({ id: "plain", priority: "critical" }),
          task({ id: "other", priority: "low" }),
        ]),
      },
      view({
        filters: [
          clause("priority", ["critical", "high"], "in"),
          clause("taskLabel", ["urgent", "missing"], "in"),
        ],
      }),
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["rich"]);
  });

  it("supports negative and substring operators", () => {
    const result = queryThreadView(
      { "workflow-1": snapshot([richTask(), task({ id: "plain", title: "Small fix" })]) },
      view({
        filters: [
          clause("titleMatch", "checkout", "matches"),
          clause("taskType", "standard", "not_matches"),
        ],
      }),
    );

    expect(result.matchingCandidates).toHaveLength(1);
    expect(result.matchingCandidates[0].taskId).toBe("rich");
  });
});

describe("queryThreadView attention sort", () => {
  it("keeps attention buckets and recency order for All threads", () => {
    const candidates = [
      task({
        id: "waiting-new",
        primarySessionState: "WAITING_FOR_INPUT",
        statusSummary: summary({
          primary_session: { id: "session-waiting-new", state: "WAITING_FOR_INPUT" },
          last_activity_at: "2026-08-31T13:00:00Z",
        }),
      }),
      task({
        id: "review-old",
        state: "REVIEW",
        primarySessionState: "COMPLETED",
        statusSummary: summary({
          primary_session: { id: "session-review-old", state: "COMPLETED" },
          last_activity_at: "2026-08-31T10:00:00Z",
        }),
      }),
      task({
        id: "running-new",
        primarySessionState: "RUNNING",
        statusSummary: summary({
          primary_session: { id: "session-running-new", state: "RUNNING" },
          last_activity_at: SAME_ACTIVITY,
        }),
      }),
      task({
        id: "action-old",
        primarySessionState: "WAITING_FOR_INPUT",
        statusSummary: summary({
          primary_session: { id: "session-action-old", state: "WAITING_FOR_INPUT" },
          pending_action: "clarification",
          last_activity_at: "2026-08-31T09:00:00Z",
        }),
      }),
      task({
        id: "action-new",
        primarySessionState: "WAITING_FOR_INPUT",
        statusSummary: summary({
          primary_session: { id: "session-action-new", state: "WAITING_FOR_INPUT" },
          pending_action: "permission",
          last_activity_at: "2026-08-31T11:00:00Z",
        }),
      }),
    ];

    const result = queryThreadView({ "workflow-1": snapshot(candidates) }, view());

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual([
      "action-new",
      "action-old",
      "running-new",
      "review-old",
      "waiting-new",
    ]);
  });
});

describe("queryThreadView sort and admission", () => {
  it("sorts deterministically and limits the admitted candidates", () => {
    const result = queryThreadView(
      {
        "workflow-1": snapshot([
          task({ id: "c", updatedAt: SAME_ACTIVITY }),
          task({ id: "a", updatedAt: SAME_ACTIVITY }),
          task({ id: "b", updatedAt: SAME_ACTIVITY }),
        ]),
      },
      view({ sort: { key: "updatedAt", direction: "desc" }, maxColumns: 2 }),
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["a", "b", "c"]);
    expect(result.admittedCandidates.map((candidate) => candidate.taskId)).toEqual(["a", "b"]);
    expect(result.matchingCount).toBe(3);
    expect(result.hiddenCount).toBe(1);
  });

  it("temporarily admits a valid hidden deep-link target without changing the view", () => {
    const activeView = view({
      taskScope: { mode: "selected", taskIds: ["first", "second", "hidden"] },
      maxColumns: 2,
    });
    const result = queryThreadView(
      {
        "workflow-1": snapshot([
          task({ id: "first" }),
          task({ id: "second" }),
          task({ id: "hidden" }),
        ]),
      },
      activeView,
      { requestedTaskId: "hidden" },
    );

    expect(result.admittedCandidates.map((candidate) => candidate.taskId)).toEqual([
      "first",
      "hidden",
    ]);
    expect(result.effectiveView).toEqual(activeView);
  });

  it("admits an eligible deep-link target outside a selected scope", () => {
    const result = queryThreadView(
      {
        "workflow-1": snapshot([task({ id: "selected" }), task({ id: "linked" })]),
      },
      view({ taskScope: { mode: "selected", taskIds: ["selected"] }, maxColumns: 1 }),
      { requestedTaskId: "linked" },
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["selected"]);
    expect(result.admittedCandidates.map((candidate) => candidate.taskId)).toEqual(["linked"]);
  });

  it("temporarily admits an eligible deep-link target hidden by a saved filter", () => {
    const result = queryThreadView(
      {
        "workflow-1": snapshot([
          task({ id: "matching", title: "Matching task" }),
          task({ id: "filtered", title: "Filtered task" }),
        ]),
      },
      view({
        filters: [clause("titleMatch", "matching", "matches")],
        maxColumns: 1,
      }),
      { requestedTaskId: "filtered" },
    );

    expect(result.matchingCandidates.map((candidate) => candidate.taskId)).toEqual(["matching"]);
    expect(result.admittedCandidates.map((candidate) => candidate.taskId)).toEqual(["filtered"]);
    expect(result.matchingCount).toBe(1);
    expect(result.temporaryAdmissionCount).toBe(1);
    expect(result.hiddenCount).toBe(1);
  });

  it("merges a matching draft without changing the saved view identity", () => {
    const activeView = view({ maxColumns: null });
    const result = queryThreadView(
      { "workflow-1": snapshot([task({ id: "one" }), task({ id: "two" })]) },
      activeView,
      {
        draft: {
          baseViewId: activeView.id,
          taskScope: { mode: "selected", taskIds: ["two"] },
          filters: [],
          sort: { key: "title", direction: "asc" },
          maxColumns: 1,
        },
      },
    );

    expect(result.admittedCandidates.map((candidate) => candidate.taskId)).toEqual(["two"]);
    expect(result.effectiveView).toMatchObject({ id: activeView.id, name: activeView.name });
    expect(result.effectiveView.maxColumns).toBe(1);
  });
});

describe("thread view fingerprint", () => {
  it("changes when query inputs change and remains stable for equivalent views", () => {
    const snapshots = { "workflow-1": snapshot([task({ id: "one" })]) };
    const first = queryThreadView(snapshots, view({ maxColumns: 2 }));
    const equivalent = queryThreadView(snapshots, view({ maxColumns: 2 }));
    const changed = queryThreadView(snapshots, view({ maxColumns: 3 }));

    expect(first.fingerprint).toBe(equivalent.fingerprint);
    expect(first.fingerprint).not.toBe(changed.fingerprint);
  });
});
