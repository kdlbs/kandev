import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { TaskGraphResponse } from "@/lib/api/domains/task-graph-api";
import { layoutTaskGraph } from "@/lib/task-graph/layout";
import { buildTaskGraphModel, traceChain } from "@/lib/task-graph/model";
import {
  XyflowTaskGraphRenderer,
  toFlowEdges,
  toFlowNodes,
  toFrameNodes,
} from "./xyflow-task-graph-renderer";
import type { TaskGraphRendererProps } from "./task-graph-renderer";

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

afterEach(cleanup);

const response: TaskGraphResponse = {
  mode: "chain",
  available_modes: ["chain"],
  focus_task_id: "b",
  nodes: ["a", "b", "c", "z"].map((id) => ({
    id,
    title: `Task ${id}`,
    state: "TODO",
    depth: 0,
    role: "member" as const,
    archived: false,
    blocked: false,
  })),
  edges: [
    { from: "a", to: "b", kind: "dependency", status: "resolved" },
    { from: "b", to: "c", kind: "dependency", status: "failed" },
  ],
  truncated: false,
};

function props(overrides: Partial<TaskGraphRendererProps> = {}): TaskGraphRendererProps {
  const model = buildTaskGraphModel(response);
  return {
    model,
    layout: layoutTaskGraph(model),
    emphasis: null,
    colorMode: "dark",
    touch: false,
    renderNode: (node) => <span>{node.title}</span>,
    nodeLabel: (node) => `label ${node.id}`,
    onNodeActivate: vi.fn(),
    onNodeEmphasize: vi.fn(),
    ...overrides,
  };
}

describe("toFlowNodes / toFlowEdges", () => {
  it("maps every node to a fixed, non-interactive xyflow node", () => {
    const p = props();
    const nodes = toFlowNodes(p);
    expect(nodes.map((n) => n.id)).toEqual(p.model.nodes.map((n) => n.id));
    for (const n of nodes) {
      expect(n).toMatchObject({
        draggable: false,
        connectable: false,
        selectable: false,
        deletable: false,
      });
      expect(n.style?.pointerEvents).toBe("all");
      expect(n.position).toEqual({ x: p.layout.get(n.id)?.x, y: p.layout.get(n.id)?.y });
    }
  });

  it("styles edges by verdict and dims edges outside the emphasized chain", () => {
    const p = props();
    const emphasis = traceChain(p.model, "a");
    const edges = toFlowEdges({ ...p, emphasis });
    const failed = edges.find((e) => e.id === "dependency:b->c");
    expect(failed?.style?.stroke).toBe("var(--destructive)");
    expect(
      edges.every((e) => e.selectable === false && e.deletable === false && e.focusable === false),
    ).toBe(true);
    // a's chain is a → b → c, so nothing is dimmed; z has no edges.
    expect(edges.every((e) => e.style?.opacity === 1)).toBe(true);
    const zChain = toFlowEdges({ ...p, emphasis: traceChain(p.model, "z") });
    expect(zChain.every((e) => e.style?.opacity !== 1)).toBe(true);
  });

  it("draws related edges dotted and without an arrow", () => {
    const relatedModel = buildTaskGraphModel({
      ...response,
      edges: [{ from: "a", to: "b", kind: "related" }],
    });
    const edge = toFlowEdges({ ...props(), model: relatedModel })[0];
    expect(edge.markerEnd).toBeUndefined();
    expect(edge.style?.strokeDasharray).toBe("2 4");
  });
});

describe("XyflowTaskGraphRenderer", () => {
  it("renders labelled node buttons that activate and emphasize", () => {
    const p = props();
    render(
      <div style={{ width: 800, height: 600 }}>
        <XyflowTaskGraphRenderer {...p} />
      </div>,
    );
    const button = screen.getByRole("button", { name: "label b" });
    fireEvent.mouseEnter(button);
    expect(p.onNodeEmphasize).toHaveBeenCalledWith("b");
    fireEvent.click(button);
    expect(p.onNodeActivate).toHaveBeenCalledWith("b");
    fireEvent.mouseLeave(button);
    expect(p.onNodeEmphasize).toHaveBeenLastCalledWith(null);
  });

  it("follows the requested color mode", () => {
    const { container } = render(<XyflowTaskGraphRenderer {...props({ colorMode: "dark" })} />);
    expect(container.querySelector(".react-flow")?.classList.contains("dark")).toBe(true);
  });

  it("emphasizes on long press without activating", () => {
    vi.useFakeTimers();
    const p = props({ touch: true });
    render(<XyflowTaskGraphRenderer {...p} />);
    const button = screen.getByRole("button", { name: "label a" });
    fireEvent.pointerDown(button, { pointerType: "touch", clientX: 0, clientY: 0 });
    vi.advanceTimersByTime(500);
    fireEvent.pointerUp(button, { pointerType: "touch" });
    fireEvent.click(button);
    expect(p.onNodeEmphasize).toHaveBeenCalledWith("a");
    expect(p.onNodeActivate).not.toHaveBeenCalled();
    vi.useRealTimers();
  });
});

describe("selection", () => {
  it("rings the selected node and opens a node on double-click", () => {
    const onNodeOpen = vi.fn();
    const p = props({
      selectedId: "b",
      onNodeOpen,
      renderNode: (n, state) => <span data-selected={state.selected}>{n.title}</span>,
    });
    render(<XyflowTaskGraphRenderer {...p} />);
    const button = screen.getByRole("button", { name: "label b" });
    expect(button.querySelector("[data-selected='true']")).not.toBeNull();
    fireEvent.doubleClick(button);
    expect(onNodeOpen).toHaveBeenCalledWith("b");
  });
});

describe("frames", () => {
  it("renders labelled background frames behind task nodes", () => {
    const p = props({
      frames: [{ id: "group:p", title: "Parent P", box: { x: 0, y: 0, width: 400, height: 200 } }],
    });
    expect(toFrameNodes(p)).toMatchObject([
      { id: "group:p", type: "frame", zIndex: -1, draggable: false, selectable: false },
    ]);
    render(<XyflowTaskGraphRenderer {...p} />);
    expect(screen.getByTestId("task-graph-frame").textContent).toBe("Parent P");
  });
});
