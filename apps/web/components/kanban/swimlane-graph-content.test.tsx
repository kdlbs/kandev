import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { ForegroundActivity, TaskPendingAction } from "@/lib/types/http";
import { i18n, t } from "@/lib/i18n";
import { SwimlaneGraphContent, moveTaskAcrossSwimlaneSteps } from "./swimlane-graph-content";

afterEach(() => {
  cleanup();
});

const STEPS: WorkflowStep[] = [{ id: "step-1", title: "In Progress", color: "#888" }];
const ICON_CHECK = ".tabler-icon-check";
const ICON_LOADER2 = ".tabler-icon-loader-2";

function makeTask(foregroundActivity?: ForegroundActivity | null): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId: "step-1",
    state: "COMPLETED",
    foregroundActivity,
  } as Task;
}

function renderSwimlane(foregroundActivity?: ForegroundActivity | null) {
  return render(
    <StateProvider>
      <SwimlaneGraphContent
        workflowId="wf-1"
        steps={STEPS}
        tasks={[makeTask(foregroundActivity)]}
        onPreviewTask={() => undefined}
      />
    </StateProvider>,
  );
}

describe("SwimlaneGraphContent — task-level background-running affordance", () => {
  it("shows the background spinner (IconLoader) for a background-running task chip, not the done check", () => {
    const { container } = renderSwimlane("background");
    // the swimlane task chip reflects the task-level
    // aggregate — background-running (IconLoader), never a done check for a task
    // still doing background work, even when the coarse state is COMPLETED.
    expect(container.querySelector(".tabler-icon-loader")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when a session is generating", () => {
    const { container } = renderSwimlane("generating");
    expect(container.querySelector(ICON_LOADER2)).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = renderSwimlane(null);
    expect(container.querySelector(ICON_CHECK)).not.toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});

describe("SwimlaneGraphContent — waiting-for-input variants", () => {
  function renderWaiting(pendingAction: TaskPendingAction) {
    const task = {
      id: "task-1",
      title: "A task",
      workflowStepId: "step-1",
      state: "WAITING_FOR_INPUT",
      primarySessionId: "session-1",
      primarySessionState: "WAITING_FOR_INPUT",
      primarySessionPendingAction: pendingAction,
    } as Task;
    return render(
      <StateProvider>
        <SwimlaneGraphContent
          workflowId="wf-1"
          steps={STEPS}
          tasks={[task]}
          onPreviewTask={() => undefined}
        />
      </StateProvider>,
    );
  }

  it("shows the message-question for a pending clarification, distinct from done and running", () => {
    const { container } = renderWaiting("clarification");
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = renderWaiting("permission");
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});

describe("moveTaskAcrossSwimlaneSteps — move failure reporting", () => {
  const WORKFLOW_ID = "wf-1";
  const TASK_ID = "task-1";
  const PLAIN_STRING_REJECTION = "plain string rejection";
  const UNKNOWN_REJECTION = {};

  function harness() {
    const snapshot = {
      tasks: [{ id: TASK_ID, workflowStepId: "step-1", position: 0 }],
    };
    const snapshots: Record<string, typeof snapshot> = { [WORKFLOW_ID]: snapshot };
    const state = {
      kanbanMulti: { snapshots },
      setWorkflowSnapshot: (id: string, next: typeof snapshot) => {
        snapshots[id] = next;
      },
    };
    return { getState: () => state } as never;
  }

  async function move(rejection: unknown) {
    const errors: { message: string }[] = [];
    await moveTaskAcrossSwimlaneSteps({
      task: { id: TASK_ID, primarySessionId: null } as never,
      taskId: TASK_ID,
      targetColumnId: "step-2",
      workflowId: WORKFLOW_ID,
      store: harness(),
      moveTaskById: (() => Promise.reject(rejection)) as never,
      onMoveError: (error) => errors.push(error),
    });
    return errors;
  }

  it("reports the localized fallback for an unknown rejection", async () => {
    await i18n.changeLanguage("pseudo");
    try {
      const [error] = await move(UNKNOWN_REJECTION);
      expect(error.message).toBe(t("task:taskMoveErrorGeneric"));
      expect(error.message).toMatch(/[^\x20-\x7E]/);
    } finally {
      await i18n.changeLanguage("en");
    }
  });

  it("attaches task identity to a string rejection", async () => {
    expect(await move(PLAIN_STRING_REJECTION)).toEqual([
      { message: PLAIN_STRING_REJECTION, taskId: TASK_ID, sessionId: null },
    ]);
  });

  it("preserves a real Error's message instead of replacing it with copy", async () => {
    // A server diagnostic is not copy: swallowing it would leave the user with a
    // generic sentence and no way to tell two failures apart.
    expect(await move(new Error("step is at its WIP limit"))).toEqual([
      { message: "step is at its WIP limit", taskId: TASK_ID, sessionId: null },
    ]);
  });
});
