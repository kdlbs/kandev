import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  agentProfileId as toAgentProfileId,
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
} from "@/lib/types/http";
import { TopbarWorkingIndicator } from "./topbar-working-indicator";
import { ActiveSessionRefProvider, useActiveSessionRef } from "./active-session-ref-context";

afterEach(() => cleanup());

const INDICATOR_TID = "topbar-working-indicator";
const T_START = "2026-05-01T10:00:00Z";
const T_UPDATE = "2026-05-01T10:01:00Z";

function liveSession(taskIdStr: string, id = "s-1"): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(taskIdStr),
    state: "RUNNING",
    started_at: T_START,
    updated_at: T_START,
  };
}

function wrap(node: ReactNode, sessions: Record<string, TaskSession>) {
  return (
    <StateProvider initialState={{ taskSessions: { items: sessions } }}>
      <ActiveSessionRefProvider>{node}</ActiveSessionRefProvider>
    </StateProvider>
  );
}

describe("TopbarWorkingIndicator", () => {
  it("renders nothing when there is no live session for the task", () => {
    render(wrap(<TopbarWorkingIndicator taskId="task-1" />, {}));
    expect(screen.queryByTestId(INDICATOR_TID)).toBeNull();
  });

  it("renders 'Working' button when a live session exists for the task", () => {
    const sessions = { "s-1": liveSession("task-1") };
    render(wrap(<TopbarWorkingIndicator taskId="task-1" />, sessions));
    expect(screen.getByTestId(INDICATOR_TID)).toBeTruthy();
    expect(screen.getByText("Working")).toBeTruthy();
    // E2E hook: a more specific data-testid only present while live.
    expect(screen.getByTestId("topbar-working-active")).toBeTruthy();
  });

  it("drops the spinner when an office session goes RUNNING → IDLE", () => {
    // Office session: agent_profile_id set + state IDLE → not live.
    const idleOffice: TaskSession = {
      id: toSessionId("s-1"),
      task_id: toTaskId("task-1"),
      agent_profile_id: toAgentProfileId("agent-a"),
      state: "IDLE",
      started_at: T_START,
      updated_at: T_UPDATE,
    };
    render(wrap(<TopbarWorkingIndicator taskId="task-1" />, { "s-1": idleOffice }));
    expect(screen.queryByTestId(INDICATOR_TID)).toBeNull();
    expect(screen.queryByTestId("topbar-working-active")).toBeNull();
  });

  it("keeps the spinner up for kanban WAITING_FOR_INPUT (no agent_profile_id)", () => {
    const waitingKanban: TaskSession = {
      id: toSessionId("s-1"),
      task_id: toTaskId("task-1"),
      state: "WAITING_FOR_INPUT",
      started_at: T_START,
      updated_at: T_UPDATE,
    };
    render(wrap(<TopbarWorkingIndicator taskId="task-1" />, { "s-1": waitingKanban }));
    expect(screen.getByTestId(INDICATOR_TID)).toBeTruthy();
  });

  it("ignores live sessions for a different task", () => {
    const sessions = { "s-1": liveSession("task-other") };
    render(wrap(<TopbarWorkingIndicator taskId="task-1" />, sessions));
    expect(screen.queryByTestId(INDICATOR_TID)).toBeNull();
  });

  it("calls scrollIntoView on the registered active node when clicked", () => {
    const sessions = { "s-1": liveSession("task-1") };
    const scrollIntoView = (() => {
      let called = false;
      const fn = () => {
        called = true;
      };
      (fn as unknown as { wasCalled: () => boolean }).wasCalled = () => called;
      return fn;
    })();

    function Anchor() {
      const { setActiveRef } = useActiveSessionRef();
      return (
        <div
          data-testid="active-anchor"
          ref={(node) => {
            if (!node) return setActiveRef("s-1", null);
            // Stub scrollIntoView on this node.
            (node as unknown as { scrollIntoView: () => void }).scrollIntoView =
              scrollIntoView as unknown as () => void;
            setActiveRef("s-1", node);
          }}
        />
      );
    }

    render(
      wrap(
        <>
          <Anchor />
          <TopbarWorkingIndicator taskId="task-1" />
        </>,
        sessions,
      ),
    );

    const button = screen.getByTestId(INDICATOR_TID);
    fireEvent.click(button);
    expect((scrollIntoView as unknown as { wasCalled: () => boolean }).wasCalled()).toBe(true);
  });
});
