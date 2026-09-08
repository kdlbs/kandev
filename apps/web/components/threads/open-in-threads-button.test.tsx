import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { HydrationState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";
import { sessionId } from "@/lib/types/ids";

const pushMock = vi.hoisted(() => vi.fn());
const pathnameMock = vi.hoisted(() => ({ value: "/t/task-1" }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: pushMock }),
  usePathname: () => pathnameMock.value,
}));

import { OpenInThreadsButton } from "./open-in-threads-button";

afterEach(() => {
  cleanup();
  pushMock.mockReset();
  pathnameMock.value = "/t/task-1";
});

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "RUNNING" as TaskSessionState,
    is_primary: true,
    started_at: "2026-08-27T10:00:00Z",
    ...overrides,
  } as TaskSession;
}

function renderButton(
  sessions: TaskSession[],
  props: Partial<ComponentProps<typeof OpenInThreadsButton>> = {},
  taskOverrides: Partial<KanbanState["tasks"][number]> = {},
) {
  const initialState = {
    kanban: {
      tasks: [
        {
          id: "task-1",
          workflowId: "workflow-1",
          workflowStepId: "step-1",
          title: "Task 1",
          position: 0,
          ...taskOverrides,
        },
      ],
    },
    taskSessionsByTask: {
      itemsByTaskId: { "task-1": sessions },
      loadingByTaskId: {},
      loadedByTaskId: { "task-1": true },
    },
  } as unknown as HydrationState;
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <OpenInThreadsButton taskId="task-1" sessionId="session-1" {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

const BUTTON_NAME = "Open in Threads";

describe("OpenInThreadsButton", () => {
  it("offers the jump for a live primary session", () => {
    renderButton([session()]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).not.toBeNull();
  });

  it("sends the deck the task to scroll to", () => {
    renderButton([session()]);

    fireEvent.click(screen.getByRole("button", { name: BUTTON_NAME }));

    expect(pushMock).toHaveBeenCalledWith("/threads?taskId=task-1&sessionId=session-1");
  });

  it("offers the jump for a settled sibling of a live primary session", () => {
    renderButton(
      [
        session(),
        session({
          id: sessionId("session-2"),
          is_primary: false,
          state: "COMPLETED" as TaskSessionState,
        }),
      ],
      { sessionId: "session-2" },
    );

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).not.toBeNull();
  });

  it("stays hidden for a session the deck has no column for", () => {
    renderButton([session({ is_primary: false })]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).toBeNull();
  });

  it("stays hidden once the agent has settled", () => {
    renderButton([session({ state: "COMPLETED" as TaskSessionState })]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).toBeNull();
  });

  it("offers the jump for a completed primary session whose task awaits review", () => {
    renderButton(
      [session({ state: "COMPLETED" as TaskSessionState })],
      {},
      { state: "REVIEW", reviewStatus: "pending" },
    );

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).not.toBeNull();
  });

  it("stays hidden inside the deck itself, where the jump is a no-op", () => {
    pathnameMock.value = "/threads";
    renderButton([session()]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).toBeNull();
  });

  it("stays hidden for the trailing-slash deck route", () => {
    pathnameMock.value = "/threads/";
    renderButton([session()]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).toBeNull();
  });

  it("stays hidden when the task's sessions are not loaded", () => {
    renderButton([]);

    expect(screen.queryByRole("button", { name: BUTTON_NAME })).toBeNull();
  });
});
