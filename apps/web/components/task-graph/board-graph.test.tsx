import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const appStore = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  listeners: new Set<() => void>(),
}));
vi.mock("@/components/state-provider", async () => {
  const { useSyncExternalStore } = await import("react");
  return {
    useAppStore: (select: (state: unknown) => unknown) =>
      useSyncExternalStore(
        (listener) => {
          appStore.listeners.add(listener);
          return () => appStore.listeners.delete(listener);
        },
        () => select(appStore.state),
        () => select(appStore.state),
      ),
  };
});
vi.mock("@/lib/task-graph/layout", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/task-graph/layout")>();
  return {
    ...actual,
    layoutGroupedTaskGraph: vi.fn(actual.layoutGroupedTaskGraph),
  };
});
vi.mock("./task-graph-canvas", () => ({
  TaskGraphCanvas: (p: {
    model: { nodes: Array<{ id: string; title: string }> };
    selectedId: string | null;
    frames?: Array<{ id: string; title: string }>;
    onActivate: (id: string) => void;
    onOpen?: (id: string) => void;
  }) => (
    <div>
      {(p.frames ?? []).map((f) => (
        <span key={f.id} data-testid="frame">
          {f.title}
        </span>
      ))}
      {p.model.nodes.map((n) => (
        <button
          key={n.id}
          type="button"
          data-selected={p.selectedId === n.id}
          onClick={() => p.onActivate(n.id)}
          onDoubleClick={() => p.onOpen?.(n.id)}
        >
          {n.title}
        </button>
      ))}
    </div>
  ),
}));

import { BoardGraph } from "./board-graph";
import { layoutGroupedTaskGraph } from "@/lib/task-graph/layout";

beforeAll(() => {
  Element.prototype.scrollIntoView ??= () => {};
});
beforeEach(() => {
  window.localStorage.clear();
  appStore.state = { kanban: { tasks: [] }, kanbanMulti: { snapshots: {} } };
  vi.mocked(layoutGroupedTaskGraph).mockClear();
});
afterEach(cleanup);

const tasks = [
  { id: "a", title: "Alpha" },
  { id: "b", title: "Beta", dependsOn: [{ id: "a" }] },
];

describe("BoardGraph", () => {
  it("selects on click, then opens or previews the selected task", () => {
    const onOpen = vi.fn();
    const onPreview = vi.fn();
    render(<BoardGraph tasks={tasks} focusId={null} onOpen={onOpen} onPreview={onPreview} />);
    expect(screen.getByText("Select a task to open it, or double-click it.")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Beta" }));
    expect(onOpen).not.toHaveBeenCalled();
    expect(screen.getByTestId("board-graph-selection").textContent).toContain("Beta");

    fireEvent.click(screen.getByTestId("board-graph-preview"));
    expect(onPreview).toHaveBeenCalledWith("b");
    fireEvent.click(screen.getByTestId("board-graph-open"));
    expect(onOpen).toHaveBeenCalledWith("b");
  });

  it("opens on double-click and preselects the focused task", () => {
    const onOpen = vi.fn();
    render(<BoardGraph tasks={tasks} focusId="a" onOpen={onOpen} onPreview={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Alpha" }).dataset.selected).toBe("true");
    fireEvent.doubleClick(screen.getByRole("button", { name: "Beta" }));
    expect(onOpen).toHaveBeenCalledWith("b");
  });

  it("swaps parent frames for parallel step bands when toggled", () => {
    const board = [
      { id: "p", title: "Parent" },
      { id: "x", title: "X", parentTaskId: "p" },
      { id: "y", title: "Y", parentTaskId: "p" },
      { id: "z", title: "Z", parentTaskId: "p", dependsOn: [{ id: "x" }] },
    ];
    render(<BoardGraph tasks={board} focusId={null} onOpen={vi.fn()} onPreview={vi.fn()} />);
    expect(screen.getAllByTestId("frame").map((f) => f.textContent)).toEqual(["Parent"]);
    fireEvent.click(screen.getByTestId("task-graph-parallel-toggle"));
    expect(screen.getAllByTestId("frame").map((f) => f.textContent)).toEqual([
      "Step 1 · 3 can run in parallel",
      "Step 2",
    ]);
  });

  it("recalculates layout when a parent title changes, but skips unrelated store updates", () => {
    appStore.state = {
      kanban: { tasks: [{ id: "parent", title: "Original parent" }] },
      kanbanMulti: { snapshots: {} },
    };
    render(
      <BoardGraph
        tasks={[{ id: "child", title: "Child", parentTaskId: "parent" }]}
        focusId={null}
        onOpen={vi.fn()}
        onPreview={vi.fn()}
      />,
    );
    expect(layoutGroupedTaskGraph).toHaveBeenCalledOnce();

    act(() => {
      appStore.state = { ...appStore.state, session: { messageCount: 1 } };
      for (const listener of appStore.listeners) listener();
    });
    expect(layoutGroupedTaskGraph).toHaveBeenCalledOnce();

    act(() => {
      appStore.state = {
        ...appStore.state,
        kanban: { tasks: [{ id: "parent", title: "Renamed parent" }] },
      };
      for (const listener of appStore.listeners) listener();
    });
    expect(layoutGroupedTaskGraph).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("frame").textContent).toBe("Renamed parent");
  });

  it("stretches the canvas to the lane height instead of a content-sized cap when fillHeight", () => {
    render(
      <BoardGraph tasks={tasks} focusId={null} onOpen={vi.fn()} onPreview={vi.fn()} fillHeight />,
    );
    const board = screen.getByTestId("board-graph");
    expect(board.className).toContain("h-full");
    const canvasWrapper = board.lastElementChild?.previousElementSibling as HTMLElement;
    expect(canvasWrapper.className).toContain("flex-1");
    expect(canvasWrapper.style.height).toBe("");
  });

  it("sizes the canvas to its content by default", () => {
    render(<BoardGraph tasks={tasks} focusId={null} onOpen={vi.fn()} onPreview={vi.fn()} />);
    const board = screen.getByTestId("board-graph");
    expect(board.className).not.toContain("h-full");
    const canvasWrapper = board.lastElementChild?.previousElementSibling as HTMLElement;
    expect(canvasWrapper.style.height).not.toBe("");
  });
});
