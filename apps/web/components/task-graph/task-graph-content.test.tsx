import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskGraphResponse } from "@/lib/api/domains/task-graph-api";
import { buildTaskGraphModel } from "@/lib/task-graph/model";
import type { UseTaskGraphResult } from "@/hooks/domains/task/use-task-graph";
import type { TaskGraphRendererProps } from "./task-graph-renderer";

const hook = vi.hoisted(() => ({ value: null as unknown as UseTaskGraphResult }));
const rendered = vi.hoisted(() => ({ props: null as TaskGraphRendererProps | null }));
const push = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/task/use-task-graph", () => ({ useTaskGraph: () => hook.value }));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push }) }));
vi.mock("./xyflow-task-graph-renderer", () => ({
  XyflowTaskGraphRenderer: (props: TaskGraphRendererProps) => {
    rendered.props = props;
    return (
      <div
        data-testid="stub-renderer"
        data-frames={(props.frames ?? []).map((f) => f.title).join("|")}
      >
        {props.model.nodes.map((n) => (
          <button key={n.id} type="button" onClick={() => props.onNodeActivate(n.id)}>
            {props.nodeLabel(n)}
          </button>
        ))}
      </div>
    );
  },
}));

import { TaskGraphContent } from "./task-graph-content";

function graph(overrides: Partial<TaskGraphResponse> = {}): TaskGraphResponse {
  return {
    mode: "subtasks",
    available_modes: ["subtasks", "chain"],
    focus_task_id: "p",
    root: { id: "p", title: "Ship billing" },
    nodes: [
      {
        id: "a",
        title: "Schema",
        state: "COMPLETED",
        depth: 1,
        role: "member",
        archived: false,
        blocked: false,
      },
      {
        id: "b",
        title: "API",
        state: "TODO",
        depth: 1,
        role: "member",
        archived: false,
        blocked: true,
      },
    ],
    edges: [{ from: "a", to: "b", kind: "dependency", status: "resolved" }],
    truncated: false,
    ...overrides,
  };
}

function setHook(overrides: Partial<UseTaskGraphResult> = {}) {
  hook.value = {
    model: buildTaskGraphModel(graph()),
    loading: false,
    error: false,
    setMode: vi.fn(),
    retry: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  push.mockReset();
  rendered.props = null;
  setHook();
});
afterEach(cleanup);

describe("TaskGraphContent", () => {
  it("shows the root name, mode selector, legend and labelled nodes", () => {
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(screen.getByText("Ship billing")).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Subtasks" }).getAttribute("aria-checked")).toBe(
      "true",
    );
    expect(screen.getByTestId("task-graph-legend")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Schema, Completed, step 1" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "API, To do, step 2" })).toBeTruthy();
  });

  it("navigates to an activated node and closes", () => {
    const onNavigate = vi.fn();
    render(<TaskGraphContent taskId="p" onNavigate={onNavigate} />);
    fireEvent.click(screen.getByRole("button", { name: /^API/ }));
    expect(push).toHaveBeenCalledWith("/t/b");
    expect(onNavigate).toHaveBeenCalled();
  });

  it("switches mode", () => {
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    fireEvent.click(screen.getByRole("radio", { name: "Chain" }));
    expect(hook.value.setMode).toHaveBeenCalledWith("chain");
  });

  it("disables Subtasks when unavailable and explains why", () => {
    setHook({
      model: buildTaskGraphModel(
        graph({ mode: "chain", available_modes: ["chain"], root: undefined }),
      ),
    });
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect((screen.getByRole("radio", { name: "Subtasks" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getByText("This task has no subtasks or parent.")).toBeTruthy();
  });

  it("shows loading, error with retry, no-order, no-dependency and truncated states", () => {
    setHook({ model: null, loading: true });
    const { rerender } = render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(screen.getByText("Loading graph")).toBeTruthy();

    setHook({ model: null, error: true });
    rerender(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(hook.value.retry).toHaveBeenCalled();

    setHook({ model: buildTaskGraphModel(graph({ edges: [], truncated: true })) });
    rerender(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(
      screen.getByText("These subtasks have no set order. They can run in parallel."),
    ).toBeTruthy();
    expect(screen.getByText("Showing the first 2 tasks.")).toBeTruthy();

    const lonely = graph({
      mode: "chain",
      available_modes: ["chain"],
      root: undefined,
      nodes: [graph().nodes[0]],
      edges: [],
    });
    setHook({ model: buildTaskGraphModel(lonely) });
    rerender(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(screen.getByText("This task has no dependencies.")).toBeTruthy();
  });

  it("shows a retry banner alongside the stale graph when a refresh fails", () => {
    setHook({ error: true });
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(screen.getByTestId("stub-renderer")).toBeTruthy();
    expect(screen.getAllByText("Could not load the graph.").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(hook.value.retry).toHaveBeenCalled();
  });

  it("passes emphasis for the hovered node to the renderer", () => {
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(rendered.props?.emphasis).toBeNull();
    act(() => rendered.props?.onNodeEmphasize("b"));
    expect([...(rendered.props?.emphasis?.nodeIds ?? [])].sort()).toEqual(["a", "b"]);
  });

  it("bands parallel steps when the toggle is on", () => {
    window.localStorage.clear();
    render(<TaskGraphContent taskId="p" onNavigate={vi.fn()} />);
    expect(screen.getByTestId("stub-renderer").dataset.frames).toBe("");
    fireEvent.click(screen.getByTestId("task-graph-parallel-toggle"));
    expect(screen.getByTestId("stub-renderer").dataset.frames).toBe("Step 1|Step 2");
  });
});
