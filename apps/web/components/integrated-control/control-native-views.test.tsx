import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ControlModelActivity } from "./control-model-activity";
import { ControlWorkspaceTaskList } from "./control-workspace-task-list";
import { NativeWorkspaceFigures } from "./control-native-metrics";
const fixture = vi.hoisted(() => ({
  readTasks: vi.fn(),
  model: {
    model: "Dynamic",
    provider: "",
    measurement_source: "provider",
    usage_completeness: "exact",
    cost_source: "unpriced",
    estimated: false,
    event_count: 1,
    tokens_in: 0,
    tokens_out: null,
    tokens_cached_read: 0,
    tokens_cached_write: null,
    tokens_total: 0,
    cost_subcents: 0,
  },
}));
vi.mock("./integrated-control-provider", () => ({
  useIntegratedControl: () => ({ data: { workspaces: [{ id: "w", name: "Workspace" }] } }),
}));
vi.mock("@/hooks/domains/coordinator/use-control-native", () => ({
  useNativeLedger: () => ({
    status: "ready",
    data: [
      {
        id: "w",
        status: "ready",
        data: {
          since: "2026-10-18T03:00:00Z",
          generated_at: "2026-10-19T03:00:00Z",
          models: [fixture.model],
        },
      },
    ],
  }),
  useNativeWorkspaceTasks: (...args: unknown[]) => {
    fixture.readTasks(...args);
    return {
      status: "ready",
      data: {
        total: 1,
        tasks: [
          {
            task_id: "task",
            workspace_id: "w",
            title: "Recorded task",
            state: "COMPLETED",
            step_name: "Done",
            session_id: "sid",
            session_state: "COMPLETED",
            archived: true,
            running_sessions: 0,
            queued_messages: 0,
            updated_at: "2026-10-19T02:00:00Z",
          },
        ],
      },
    };
  },
}));
afterEach(cleanup);
describe("native control views", () => {
  it("distinguishes measured zero, nullable tokens, unpriced cost and a policy model", () => {
    render(<ControlModelActivity workspaceId="w" />);
    expect(screen.getByTestId("control-ledger-tokens_in").textContent).toBe("0");
    expect(screen.getByTestId("control-ledger-tokens_cached_read").textContent).toBe("0");
    // A column no record reported is not drawn at all rather than filled with Unknown.
    expect(screen.queryByTestId("control-ledger-tokens_out")).toBeNull();
    expect(screen.getByTestId("control-ledger-cost").textContent).toBe("Unknown");
    expect(screen.queryByText("Dynamic")).toBeNull();
  });
  it("includes archived completion records and labels update time while retaining the SID link", () => {
    render(
      <ControlWorkspaceTaskList workspaceId="w" filter="completed" supported onFilter={() => {}} />,
    );
    expect(fixture.readTasks.mock.calls.at(-1)?.slice(0, 4)).toEqual(["w", "completed", "all", 50]);
    expect(screen.getByRole("link", { name: "Recorded task" }).getAttribute("href")).toBe(
      "/t/task?sessionId=sid",
    );
    expect(screen.getByText(/^Updated:/).getAttribute("dateTime")).toBe("2026-10-19T02:00:00Z");
    fireEvent.click(screen.getByRole("button", { name: "Unarchived" }));
    expect(fixture.readTasks.mock.calls.at(-1)?.[2]).toBe("active");
  });
  it("keeps unconfirmed measurement and cost sources unknown even when stored totals are zero", () => {
    const original = { ...fixture.model };
    try {
      fixture.model.measurement_source = "unknown";
      fixture.model.cost_source = "unavailable";
      render(<ControlModelActivity />);
      expect(screen.queryByText("Input tokens")).toBeNull();
      expect(screen.queryByText("Total tokens")).toBeNull();
      expect(screen.getByTestId("control-ledger-cost").textContent).toBe("Unknown");
    } finally {
      Object.assign(fixture.model, original);
    }
  });
  it("keeps process-session counts distinct from task filters and live inference", () => {
    const onFilter = vi.fn();
    render(
      <NativeWorkspaceFigures
        data={{
          workspace_id: "w",
          since: "2026-10-18T03:00:00Z",
          generated_at: "2026-10-19T03:00:00Z",
          basis: "recorded_task_session_queue_transition_state",
          inference_state: "unknown",
          tasks: { IN_PROGRESS: 2 },
          sessions: { RUNNING: 3, FAILED: 1, WAITING_FOR_INPUT: 7 },
          waiting_input_tasks: 2,
          wip_waiting_tasks: 0,
          queued_messages: 0,
          transitions_24h: 4,
          completed_tasks_24h: 1,
        }}
        filter={null}
        onFilter={onFilter}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Process sessions running/ }));
    expect(onFilter).toHaveBeenCalledWith("sessions");
    expect(screen.getByText("Failed process sessions").closest("button")).toBeNull();
    const waiting = screen.getByText("Waiting for input").parentElement;
    expect(waiting?.textContent).toContain("2");
    expect(waiting?.textContent).not.toContain("7");
  });
});
