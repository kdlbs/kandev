import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const env = vi.hoisted(() => ({
  search: "panel=task-graph&sessionId=s1",
  activeTaskId: "t1",
  api: { getPanel: () => undefined } as unknown,
  restoring: false,
  layoutEnvId: null as string | null,
  session: null as null | { id: string; task_id: string; env?: string },
  addTaskGraphPanel: vi.fn(),
  replace: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: env.replace }),
  useSearchParams: () => new URLSearchParams(env.search),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (s: unknown) => unknown) =>
    sel({
      tasks: { activeTaskId: env.activeTaskId, activeSessionId: env.session?.id ?? null },
      taskSessions: { items: env.session ? { [env.session.id]: env.session } : {} },
      environmentIdBySessionId: env.session?.env ? { [env.session.id]: env.session.env } : {},
    }),
}));
vi.mock("@/lib/state/dockview-store", () => {
  const state = () => ({
    api: env.api,
    isRestoringLayout: env.restoring,
    currentLayoutEnvId: env.layoutEnvId,
    addTaskGraphPanel: env.addTaskGraphPanel,
  });
  const hook = (sel: (s: ReturnType<typeof state>) => unknown) => sel(state());
  hook.getState = state;
  return { useDockviewStore: hook };
});
vi.mock("./task-graph-content", () => ({
  TaskGraphContent: () => <div data-testid="graph-body" />,
}));

import { TaskGraphPanelRequest } from "./task-graph-panel-request";
import {
  captureTaskGraphFocusTarget,
  takePendingTaskGraphFocusTarget,
} from "@/hooks/domains/task/use-open-task-graph";

beforeEach(() => {
  takePendingTaskGraphFocusTarget();
  vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame"] });
  Object.assign(env, {
    search: "panel=task-graph&sessionId=s1",
    activeTaskId: "t1",
    api: { getPanel: () => undefined },
    restoring: false,
    layoutEnvId: null,
    session: null,
  });
  env.addTaskGraphPanel.mockReset();
  env.replace.mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const flushFrame = () => act(() => void vi.advanceTimersToNextFrame());

describe("TaskGraphPanelRequest", () => {
  it("opens the workbench tab and drops the param", () => {
    render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).toHaveBeenCalledOnce();
    expect(env.replace).toHaveBeenCalledWith("/t/t1?sessionId=s1");
  });

  it("waits while the layout restores or another task is still active", () => {
    env.restoring = true;
    const { rerender } = render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    env.restoring = false;
    env.activeTaskId = "previous";
    rerender(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).not.toHaveBeenCalled();
  });

  it("waits until the routed task's environment layout is applied", () => {
    env.session = { id: "s1", task_id: "t1", env: "env-1" };
    env.layoutEnvId = "env-previous";
    const { rerender } = render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).not.toHaveBeenCalled();
    env.layoutEnvId = "env-1";
    rerender(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).toHaveBeenCalledOnce();
  });

  it("ignores the previous task's session while navigating", () => {
    env.session = { id: "s0", task_id: "t0", env: "env-0" };
    env.layoutEnvId = "env-0";
    render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).not.toHaveBeenCalled();
  });

  it("re-adds the tab once when the task's new environment layout is adopted", () => {
    const { rerender } = render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).toHaveBeenCalledOnce();
    // Session ensure creates the environment and the workbench adopts its layout.
    env.search = "sessionId=s1";
    env.session = { id: "s1", task_id: "t1", env: "env-new" };
    env.layoutEnvId = "env-new";
    rerender(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    expect(env.addTaskGraphPanel).toHaveBeenCalledTimes(2);
    rerender(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    expect(env.addTaskGraphPanel).toHaveBeenCalledTimes(2);
  });

  it("opens the sheet on phones", async () => {
    render(<TaskGraphPanelRequest taskId="t1" usesDockview={false} />);
    flushFrame();
    expect(await screen.findByTestId("graph-body")).toBeTruthy();
    expect(env.addTaskGraphPanel).not.toHaveBeenCalled();
  });

  it("returns focus to the phone opener when the sheet closes", async () => {
    const opener = document.createElement("button");
    document.body.append(opener);
    opener.focus();
    captureTaskGraphFocusTarget();

    render(<TaskGraphPanelRequest taskId="t1" usesDockview={false} />);
    flushFrame();
    await screen.findByTestId("graph-body");
    fireEvent.click(screen.getByTestId("task-graph-close"));
    await waitFor(() => expect(document.activeElement).toBe(opener));
    opener.remove();
  });

  it("does nothing without the param", () => {
    env.search = "sessionId=s1";
    render(<TaskGraphPanelRequest taskId="t1" usesDockview />);
    flushFrame();
    expect(env.addTaskGraphPanel).not.toHaveBeenCalled();
    expect(env.replace).not.toHaveBeenCalled();
  });
});
