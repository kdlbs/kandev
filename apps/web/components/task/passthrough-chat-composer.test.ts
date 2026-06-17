import { describe, expect, it, vi } from "vitest";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { DiffComment } from "@/lib/diff/types";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import {
  buildContextFilesMeta,
  buildPassthroughFinalMessage,
  clearPassthroughComposerContext,
  formatPassthroughBaseMessage,
} from "./passthrough-chat-composer";

const SESSION_ID = "session-1";

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
    contextFiles: [],
    prompts: [],
    pendingPRFeedback: [],
    planComments: [],
    planModeEnabled: false,
    handleClearPRFeedback: vi.fn(),
    clearSessionPlanComments: vi.fn(),
    clearEphemeral: vi.fn(),
    addContextFile: vi.fn(),
    ...overrides,
  } as never;
}

describe("passthrough chat composer helpers", () => {
  it("filters virtual prompt and plan context files from context_files metadata", () => {
    expect(
      buildContextFilesMeta([
        file("src/app.ts", "app.ts"),
        file("prompt:review", "Review prompt"),
        file("plan:context", "Plan"),
      ]),
    ).toEqual([{ path: "src/app.ts", name: "app.ts" }]);

    expect(buildContextFilesMeta([file("prompt:review"), file("plan:context")])).toBeUndefined();
  });

  it("prepends pending review comments when no structured comment payload is supplied", () => {
    const result = formatPassthroughBaseMessage("Ship it", undefined, [comment()], panelState());

    expect(result.commentsToSend).toHaveLength(1);
    expect(result.formatted).toContain("### Review Comments");
    expect(result.formatted).toContain("Please fix this");
    expect(result.formatted).toContain("Ship it");
  });

  it("merges selected context files with inline file, prompt, and task mentions", () => {
    const inlineTask: TaskMentionData = {
      taskId: "task-2",
      title: "Follow-up task",
      workflowId: "workflow-1",
      workflowStepId: "step-1",
      state: "in_progress",
    };
    const result = buildPassthroughFinalMessage({
      content: "Please check this",
      pendingComments: [],
      panelState: panelState({
        contextFiles: [file("src/existing.ts", "existing.ts")],
        prompts: [{ id: "prompt-1", name: "Review", content: "Look carefully." }],
      }),
      inlineMentions: [file("src/inline.ts", "inline.ts"), file("prompt:prompt-1", "Review")],
      inlineTaskMentions: [inlineTask],
      getState: () =>
        ({
          kanban: { steps: [{ id: "step-1", title: "Review" }] },
          kanbanMulti: { snapshots: {} },
        }) as never,
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

  it("clears ephemeral context and re-adds plan context when plan mode stays enabled", () => {
    const state = panelState({
      planModeEnabled: true,
      pendingPRFeedback: [{ id: "feedback-1" }],
      planComments: [{ id: "plan-comment-1" }],
    }) as unknown as {
      handleClearPRFeedback: ReturnType<typeof vi.fn>;
      clearSessionPlanComments: ReturnType<typeof vi.fn>;
      clearEphemeral: ReturnType<typeof vi.fn>;
      addContextFile: ReturnType<typeof vi.fn>;
    };

    clearPassthroughComposerContext(state as never);

    expect(state.handleClearPRFeedback).toHaveBeenCalledTimes(1);
    expect(state.clearSessionPlanComments).toHaveBeenCalledTimes(1);
    expect(state.clearEphemeral).toHaveBeenCalledWith(SESSION_ID);
    expect(state.addContextFile).toHaveBeenCalledWith(SESSION_ID, {
      path: "plan:context",
      name: "Plan",
    });
  });
});
