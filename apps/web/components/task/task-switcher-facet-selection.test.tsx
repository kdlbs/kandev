import { useReducer, type MouseEvent, type ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { INITIAL_STATE, multiSelectReducer } from "@/hooks/use-task-multi-select";
import type { TaskSelectionRange } from "@/hooks/use-task-multi-select";
import { flattenVisibleTaskOccurrences } from "@/lib/sidebar/apply-view";
import { groupSidebarTasksByFacet } from "@/lib/sidebar/sidebar-facet-groups";
import type { TaskSwitcherItem } from "./task-switcher-types";
import { useSelectionHandlers } from "./task-session-sidebar-selection";

vi.mock("./task-item", () => ({
  TaskItem: ({
    title,
    onSelect,
    isMultiSelected,
  }: {
    title: string;
    onSelect?: (event: MouseEvent<HTMLButtonElement>) => void;
    isMultiSelected?: boolean;
  }) => (
    <button onClick={onSelect} aria-pressed={isMultiSelected}>
      {title}
    </button>
  ),
}));
vi.mock("./task-switcher-context-menu", () => ({
  TaskItemWithContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./task-switcher-subtask-dnd", () => ({
  SortableTaskLevel: ({
    tasks,
    renderNode,
  }: {
    tasks: TaskSwitcherItem[];
    renderNode: (task: TaskSwitcherItem, draggable: boolean) => ReactNode;
  }) => <>{tasks.map((task) => renderNode(task, false))}</>,
  SortableTaskNode: ({ handle, nested }: { handle: ReactNode; nested?: ReactNode }) => (
    <>
      {handle}
      {nested}
    </>
  ),
  TaskTreeDndGroup: ({ children }: { children: (targets: Set<string>) => ReactNode }) => (
    <>{children(new Set())}</>
  ),
  NestDropZone: () => null,
}));

import { TaskSwitcher } from "./task-switcher";

const FACET = "facet:selection-test:tags";
const alpha = { value: "alpha", label: "Alpha" };
const beta = { value: "beta", label: "Beta" };
const grouped = groupSidebarTasksByFacet(
  [
    { id: "b", title: "B" },
    { id: "a", title: "A" },
    { id: "x", title: "X" },
  ],
  FACET,
  {
    [`${FACET}:a`]: [alpha, beta],
    [`${FACET}:b`]: [beta],
    [`${FACET}:x`]: [alpha],
  },
);
const occurrences = flattenVisibleTaskOccurrences(grouped, [], []);
const visibleTaskIds = occurrences.map((row) => row.taskId);
const visibleOccurrenceKeys = occurrences.map((row) => row.key);

afterEach(cleanup);

function SelectionSurface({ bulkDelete }: { bulkDelete: (ids: string[]) => void }) {
  const [state, dispatch] = useReducer(multiSelectReducer, INITIAL_STATE);
  const multiSelect = {
    isSelecting: state.selectedIds.size > 0,
    clearSelection: () => dispatch({ type: "set_selected", ids: new Set() }),
    selectRange: (taskId: string, orderedIds: string[], occurrence?: TaskSelectionRange) =>
      dispatch({ type: "select_range", taskId, orderedIds, occurrence }),
    bulkMove: vi.fn(),
  } as unknown as Parameters<typeof useSelectionHandlers>[0]["multiSelect"];
  const handlers = useSelectionHandlers({
    multiSelect,
    pinTasks: vi.fn(),
    unpinTasks: vi.fn(),
    pinnedTaskIds: [],
    visibleTaskIds,
    visibleOccurrenceKeys,
    movableSelectedIds: new Set(visibleTaskIds),
  });
  return (
    <>
      <TaskSwitcher
        grouped={grouped}
        activeTaskId={null}
        selectedTaskId={null}
        onSelectTask={vi.fn()}
        selectedTaskIds={state.selectedIds}
        onToggleSelectTask={(taskId, occurrenceId) =>
          dispatch({ type: "toggle_select", taskId, occurrenceId })
        }
        onSelectTaskRange={handlers.onSelectTaskRange}
      />
      <button onClick={() => bulkDelete([...state.selectedIds])}>Delete selected</button>
    </>
  );
}

describe("facet row range selection", () => {
  it.each(["target", "anchor"])(
    "uses the later repeated task as the range %s without selecting an intervening group",
    (endpoint) => {
      const bulkDelete = vi.fn();
      render(<SelectionSurface bulkDelete={bulkDelete} />);
      const repeated = screen.getAllByRole("button", { name: "A" });
      const b = screen.getByRole("button", { name: "B" });
      fireEvent.click(endpoint === "target" ? b : repeated[1], { ctrlKey: true });
      fireEvent.click(endpoint === "target" ? repeated[1] : b, { shiftKey: true });
      expect(b.getAttribute("aria-pressed")).toBe("true");
      expect(repeated.every((row) => row.getAttribute("aria-pressed") === "true")).toBe(true);
      expect(screen.getByRole("button", { name: "X" }).getAttribute("aria-pressed")).toBe("false");
      fireEvent.click(screen.getByRole("button", { name: "Delete selected" }));
      const ids = bulkDelete.mock.calls[0][0];
      expect(new Set(ids)).toEqual(new Set(["a", "b"]));
      expect(ids).toHaveLength(2);
    },
  );

  it("deduplicates a task when a rendered range includes both copies", () => {
    const bulkDelete = vi.fn();
    render(<SelectionSurface bulkDelete={bulkDelete} />);
    const repeated = screen.getAllByRole("button", { name: "A" });
    fireEvent.click(repeated[0], { ctrlKey: true });
    fireEvent.click(repeated[1], { shiftKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Delete selected" }));
    expect(bulkDelete).toHaveBeenCalledWith(["a", "x", "b"]);
  });

  it("realigns a deselected anchor to the surviving task's visible occurrence", () => {
    const bulkDelete = vi.fn();
    render(<SelectionSurface bulkDelete={bulkDelete} />);
    const repeated = screen.getAllByRole("button", { name: "A" });
    const b = screen.getByRole("button", { name: "B" });
    fireEvent.click(repeated[0], { ctrlKey: true });
    fireEvent.click(b, { ctrlKey: true });
    fireEvent.click(b, { ctrlKey: true });
    fireEvent.click(repeated[1], { shiftKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Delete selected" }));
    expect(bulkDelete).toHaveBeenCalledWith(["a", "x", "b"]);
  });
});
