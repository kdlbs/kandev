import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { OverviewLast24h } from "./control-history-list";
import { occurredTime } from "./overview-format";
import type { OverviewEvent } from "@/lib/types/control-overview";

afterEach(cleanup);
const EVENT_ROW = "overview-event";
const FAILED_FILTER = "overview-event-kind-session_failed";

describe("reused last 24 hours list", () => {
  it("applies the row cap after selecting an event kind and preserves task/SID navigation", () => {
    const events: OverviewEvent[] = Array.from({ length: 55 }, (_, index) => ({
      kind: "task_created",
      at: "2026-10-18T13:00:00Z",
      task_id: `task-${index}`,
      title: `Created ${index}`,
    }));
    events.push({
      kind: "session_failed",
      at: "2026-10-18T12:00:00Z",
      task_id: "failed-task",
      session_id: "exact-session",
      title: "Failure outside initial cap",
      workspace_id: "workspace-one",
    });
    render(
      <OverviewLast24h events={events} workspaceNames={{ "workspace-one": "Workspace One" }} />,
    );
    expect(screen.getAllByTestId(EVENT_ROW)).toHaveLength(50);
    expect(screen.getByTestId("overview-event-visible-count").textContent).toBe("50 of 56");
    fireEvent.click(screen.getByTestId(FAILED_FILTER));
    const row = screen.getByTestId(EVENT_ROW);
    expect(row.textContent).toContain("Failure outside initial cap");
    expect(row.getAttribute("href")).toBe("/t/failed-task?sessionId=exact-session");
    expect(screen.getByTestId(FAILED_FILTER).getAttribute("aria-pressed")).toBe("true");
  });
  it("preserves an explicit filter when refreshed events no longer contain that kind", () => {
    const event: OverviewEvent = {
      kind: "session_failed",
      at: "2026-10-18T12:00:00Z",
      title: "Failed",
      task_id: "failed",
    };
    const view = render(<OverviewLast24h events={[event]} workspaceNames={{}} />);
    fireEvent.click(screen.getByTestId(FAILED_FILTER));
    view.rerender(
      <OverviewLast24h
        events={[{ ...event, kind: "task_created", title: "New" }]}
        workspaceNames={{}}
      />,
    );
    expect(screen.queryByTestId(EVENT_ROW)).toBeNull();
    expect(screen.getByTestId(FAILED_FILTER).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId(FAILED_FILTER).textContent).toContain("0");
    expect(screen.getByTestId("overview-event-kind-all").getAttribute("aria-pressed")).toBe(
      "false",
    );
  });
  it("retains existing automation detail navigation", () => {
    render(
      <OverviewLast24h
        events={[
          {
            kind: "automation_run",
            at: "2026-10-18T12:00:00Z",
            workspace_id: "w",
            automation_id: "a",
            title: "Automation",
          },
        ]}
        workspaceNames={{ w: "Workspace" }}
      />,
    );
    expect(screen.getByTestId(EVENT_ROW).getAttribute("href")).toBe(
      "/settings/workspaces/w/automations/a",
    );
  });
  it("preserves empty list state and a fixed time across midnight", () => {
    render(<OverviewLast24h events={[]} workspaceNames={{}} />);
    expect(screen.queryByTestId(EVENT_ROW)).toBeNull();
    const now = new Date(2026, 9, 18, 0, 1).getTime();
    const yesterday = new Date(2026, 9, 17, 23, 59).toISOString();
    expect(occurredTime(yesterday, now)).toContain("10-17");
    expect(occurredTime("invalid", now)).toBe("");
  });
});

describe("history list folding", () => {
  it("hides routine records until asked, names untitled office runs and folds step moves", () => {
    const moves: OverviewEvent[] = Array.from({ length: 3 }, (_, index) => ({
      id: `transition:${index}`,
      kind: "step_move",
      at: `2026-10-18T12:0${3 - index}:00Z`,
      task_id: "looping",
      title: "Looping task",
    }));
    const routine: OverviewEvent = {
      id: "activity:0f8c2b8e-5d43-4a77-9c0e-1f6c8f1d2a3b",
      kind: "run_idle_skipped",
      at: "2026-10-18T11:00:00Z",
    };
    render(
      <OverviewLast24h
        events={[...moves, routine]}
        workspaceNames={{}}
        quietKinds={["run_idle_skipped"]}
      />,
    );
    expect(screen.getAllByTestId("control-history-move-run")).toHaveLength(1);
    expect(screen.queryByTestId(EVENT_ROW)).toBeNull();
    fireEvent.click(screen.getByTestId("overview-event-quiet"));
    const row = screen.getByTestId(EVENT_ROW);
    expect(row.textContent).toContain("Office automatic run");
    expect(row.textContent).not.toContain("activity:");
  });
});
