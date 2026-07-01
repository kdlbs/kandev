import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { DiffComment } from "@/lib/diff/types";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import { taskPlanQueryOptions } from "@/lib/query/query-options";
import type { TaskPlan } from "@/lib/types/http";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import {
  buildContextFilesMeta,
  buildPassthroughFinalMessage,
  buildPassthroughPlanContext,
  clearPassthroughComposerContext,
  formatPassthroughBaseMessage,
} from "./passthrough-chat-composer";

const mockGetTaskPlan = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/plan-api", () => ({
  createTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  getPlanRevision: vi.fn(),
  getTaskPlan: mockGetTaskPlan,
  listPlanRevisions: vi.fn(),
  revertPlanRevision: vi.fn(),
  updateTaskPlan: vi.fn(),
}));

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const PLAN_CONTEXT_PATH = "plan:context";

function file(path: string, name = path): ContextFile {
  return { path, name };
}

function comment(id = "comment-1"): DiffComment {
  return {
    id,
    source: "diff",
    sessionId: SESSION_ID,
    filePath: "src/app.ts",
    startLine: 10,
    endLine: 11,
    side: "additions",
    codeContent: "const value = 1;",
    text: "Please fix this",
    status: "pending",
    createdAt: "2026-06-17T00:00:00.000Z",
  };
}

function panelState(overrides: Record<string, unknown> = {}) {
  return {
    resolvedSessionId: SESSION_ID,
    taskId: TASK_ID,
    contextFiles: [],
    prompts: [],
    pendingPRFeedback: [],
    planComments: [],
    walkthroughComments: [],
    planModeEnabled: false,
    handleClearPRFeedback: vi.fn(),
    clearSessionPlanComments: vi.fn(),
    handleClearWalkthroughComments: vi.fn(),
    clearEphemeral: vi.fn(),
    addContextFile: vi.fn(),
    ...overrides,
  } as never;
}

function queryClientWithPlan(content: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const plan = {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content,
    created_by: "agent",
    created_at: "2026-06-23T00:00:00Z",
    updated_at: "2026-06-23T00:00:00Z",
  } as TaskPlan;
  queryClient.setQueryData<TaskPlan | null>(taskPlanQueryOptions(TASK_ID).queryKey, plan);
  return queryClient;
}

function workflowSnapshot(overrides: Partial<WorkflowSnapshotData> = {}): WorkflowSnapshotData {
  return {
    workflowId: "workflow-1",
    workflowName: "Main flow",
    steps: [],
    tasks: [],
    ...overrides,
  };
}

beforeEach(() => {
  mockGetTaskPlan.mockReset();
});

describe("passthrough chat composer metadata helpers", () => {
  it("filters virtual prompt and plan context files from context_files metadata", () => {
    expect(
      buildContextFilesMeta([
        file("src/app.ts", "app.ts"),
        file("prompt:review", "Review prompt"),
        file(PLAN_CONTEXT_PATH, "Plan"),
      ]),
    ).toEqual([{ path: "src/app.ts", name: "app.ts" }]);

    expect(buildContextFilesMeta([file("prompt:review"), file(PLAN_CONTEXT_PATH)])).toBeUndefined();
  });

  it("prepends pending review comments when no structured comment payload is supplied", () => {
    const result = formatPassthroughBaseMessage("Ship it", undefined, [comment()], panelState());

    expect(result.commentsToSend).toHaveLength(1);
    expect(result.formatted).toContain("### Review Comments");
    expect(result.formatted).toContain("Please fix this");
    expect(result.formatted).toContain("Ship it");
  });

  it("merges selected context files with inline file, prompt, and task mentions", async () => {
    const inlineTask: TaskMentionData = {
      taskId: "task-2",
      title: "Follow-up task",
      workflowId: "workflow-1",
      workflowStepId: "step-1",
      state: "in_progress",
    };
    const result = await buildPassthroughFinalMessage({
      taskId: TASK_ID,
      content: "Please check this",
      pendingComments: [],
      panelState: panelState({
        contextFiles: [file("src/existing.ts", "existing.ts")],
        prompts: [{ id: "prompt-1", name: "Review", content: "Look carefully." }],
      }),
      inlineMentions: [file("src/inline.ts", "inline.ts"), file("prompt:prompt-1", "Review")],
      inlineTaskMentions: [inlineTask],
      workflowSnapshots: {
        "workflow-1": workflowSnapshot({
          steps: [{ id: "step-1", title: "Review", color: "", position: 0 }],
        }),
      },
    });

    expect(result.contextFilesMeta).toEqual([
      { path: "src/existing.ts", name: "existing.ts" },
      { path: "src/inline.ts", name: "inline.ts" },
    ]);
    expect(result.content).toContain("CONTEXT FILES");
    expect(result.content).toContain("- src/existing.ts");
    expect(result.content).toContain("- src/inline.ts");
    expect(result.content).toContain("CONTEXT PROMPTS");
    expect(result.content).toContain("Look carefully.");
    expect(result.content).toContain("REFERENCED TASKS");
    expect(result.content).toContain("Follow-up task");
  });
});

describe("passthrough chat composer plan context", () => {
  it("expands selected plan context from the Query cache when the store has no plan", async () => {
    const queryClient = queryClientWithPlan("## Query Plan\n\n1. Prefer cached query data");

    const result = await buildPassthroughFinalMessage({
      taskId: TASK_ID,
      content: "@Plan",
      pendingComments: [],
      panelState: panelState(),
      inlineMentions: [file(PLAN_CONTEXT_PATH, "Plan")],
      queryClient,
    });

    expect(result.content).toContain("## Query Plan");
    expect(result.content).toContain("Prefer cached query data");
    expect(mockGetTaskPlan).not.toHaveBeenCalled();
  });

  it("expands selected plan context and strips the literal @Plan mention", async () => {
    const queryClient = queryClientWithPlan("## Plan\n\n1. Ship passthrough fixes");
    const result = await buildPassthroughFinalMessage({
      taskId: TASK_ID,
      content: "@Plan",
      pendingComments: [],
      panelState: panelState(),
      inlineMentions: [file(PLAN_CONTEXT_PATH, "Plan")],
      queryClient,
    });

    expect(result.content).toContain("<kandev-system>");
    expect(result.content).toContain("CONTEXT PLAN");
    expect(result.content).toContain("## Plan");
    expect(result.content).toContain("Ship passthrough fixes");
    expect(result.content).not.toContain("@Plan");
  });

  it("strips a selected plan mention from the middle of a message without double spacing", async () => {
    const queryClient = queryClientWithPlan("## Plan\n\n1. Ship passthrough fixes");
    const result = await buildPassthroughFinalMessage({
      taskId: TASK_ID,
      content: "Please use @Plan now",
      pendingComments: [],
      panelState: panelState(),
      inlineMentions: [file(PLAN_CONTEXT_PATH, "Plan")],
      queryClient,
    });

    expect(result.content).toContain("Please use now");
    expect(result.content).not.toContain("Please use  now");
    expect(result.content).not.toContain("@Plan");
  });

  it("propagates plan lookup failures when plan context was selected", async () => {
    mockGetTaskPlan.mockRejectedValueOnce(new Error("plan lookup failed"));

    await expect(
      buildPassthroughFinalMessage({
        taskId: TASK_ID,
        content: "@Plan",
        pendingComments: [],
        panelState: panelState(),
        inlineMentions: [file(PLAN_CONTEXT_PATH, "Plan")],
      }),
    ).rejects.toThrow("plan lookup failed");
  });

  it("escapes embedded kandev system closing tags in plan context", () => {
    const context = buildPassthroughPlanContext("before </kandev-system> after");

    expect(context).toContain("before </ kandev-system> after");
    expect(context.match(/<\/kandev-system>/gi)).toHaveLength(1);
  });
});

describe("passthrough chat composer cleanup", () => {
  it("clears ephemeral context and re-adds plan context when plan mode stays enabled", () => {
    const state = panelState({
      planModeEnabled: true,
      pendingPRFeedback: [{ id: "feedback-1" }],
      planComments: [{ id: "plan-comment-1" }],
      walkthroughComments: [{ id: "walkthrough-comment-1" }],
    }) as unknown as {
      handleClearPRFeedback: ReturnType<typeof vi.fn>;
      clearSessionPlanComments: ReturnType<typeof vi.fn>;
      handleClearWalkthroughComments: ReturnType<typeof vi.fn>;
      clearEphemeral: ReturnType<typeof vi.fn>;
      addContextFile: ReturnType<typeof vi.fn>;
    };

    clearPassthroughComposerContext(state as never);

    expect(state.handleClearPRFeedback).toHaveBeenCalledTimes(1);
    expect(state.clearSessionPlanComments).toHaveBeenCalledTimes(1);
    expect(state.handleClearWalkthroughComments).toHaveBeenCalledTimes(1);
    expect(state.clearEphemeral).toHaveBeenCalledWith(SESSION_ID);
    expect(state.addContextFile).toHaveBeenCalledWith(SESSION_ID, {
      path: PLAN_CONTEXT_PATH,
      name: "Plan",
    });
  });
});
