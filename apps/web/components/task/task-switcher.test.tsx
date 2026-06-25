import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";

afterEach(() => cleanup());

function Providers({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={queryClient}>
      <StateProvider>
        <ToastProvider>{children}</ToastProvider>
      </StateProvider>
    </QueryClientProvider>
  );
}

function item(id: string, parentTaskId?: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", parentTaskId };
}

// root → child → grandchild (depth 2)
const ROOT = item("Root");
const CHILD = item("Child", "Root");
const GRANDCHILD = item("Grandchild", "Child");

function grouped(): GroupedSidebarList {
  return {
    groups: [{ key: "__all__", label: "All", tasks: [ROOT] }],
    subTasksByParentId: new Map([
      ["Root", [CHILD]],
      ["Child", [GRANDCHILD]],
    ]),
  };
}

function renderSwitcher(collapsedSubtaskParentIds: string[] = []) {
  return render(
    <Providers>
      <TaskSwitcher
        grouped={grouped()}
        activeTaskId={null}
        selectedTaskId={null}
        onSelectTask={vi.fn()}
        onToggleSubtasks={vi.fn()}
        collapsedSubtaskParentIds={collapsedSubtaskParentIds}
      />
    </Providers>,
  );
}

function blockDepth(container: HTMLElement, taskId: string): string | null {
  return (
    container
      .querySelector(`[data-testid='sortable-task-block'][data-task-id='${taskId}']`)
      ?.getAttribute("data-depth") ?? null
  );
}

describe("TaskSwitcher — nested subtasks beyond depth 1", () => {
  it("renders the full tree (root, child, grandchild)", () => {
    renderSwitcher();
    expect(screen.queryByText("Root")).not.toBeNull();
    expect(screen.queryByText("Child")).not.toBeNull();
    expect(screen.queryByText("Grandchild")).not.toBeNull();
  });

  it("tags each row with its tree depth", () => {
    const { container } = renderSwitcher();
    expect(blockDepth(container, "Root")).toBe("0");
    expect(blockDepth(container, "Child")).toBe("1");
    expect(blockDepth(container, "Grandchild")).toBe("2");
  });

  it("collapsing a mid-level parent hides its whole subtree", () => {
    renderSwitcher(["Child"]);
    expect(screen.queryByText("Root")).not.toBeNull();
    expect(screen.queryByText("Child")).not.toBeNull();
    // Grandchild lives under the collapsed Child, so it must not render.
    expect(screen.queryByText("Grandchild")).toBeNull();
  });

  it("group header counts the whole subtree, not just direct children", () => {
    renderSwitcher();
    // Header only shows for >1 group or a non-default key. Force it by using a
    // keyed group instead of the implicit __all__ bucket.
    cleanup();
    render(
      <Providers>
        <TaskSwitcher
          grouped={{
            groups: [{ key: "wf1", label: "Workflow 1", tasks: [ROOT] }],
            subTasksByParentId: grouped().subTasksByParentId,
          }}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onToggleSubtasks={vi.fn()}
          collapsedSubtaskParentIds={[]}
        />
      </Providers>,
    );
    // Root + Child + Grandchild = 3
    expect(screen.queryByText("3")).not.toBeNull();
  });

  it("parent subtask toggle counts all descendants, not just direct children", () => {
    const { container } = renderSwitcher();
    const rootToggle = container.querySelector(
      "[data-testid='sidebar-subtask-toggle'][data-task-id='Root']",
    );
    expect(rootToggle).not.toBeNull();
    // Root → Child → Grandchild: badge should reflect 2 hidden rows, not 1.
    expect(rootToggle!.textContent).toContain("2");
  });

  it("omits grab cursor on nested rows when subtask reorder is disabled", () => {
    const { container } = render(
      <Providers>
        <TaskSwitcher
          grouped={grouped()}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onToggleSubtasks={vi.fn()}
          collapsedSubtaskParentIds={[]}
        />
      </Providers>,
    );
    for (const taskId of ["Child", "Grandchild"]) {
      const handle = container.querySelector(
        `[data-testid='sortable-task-block'][data-task-id='${taskId}'] > [data-testid='sortable-task-handle']`,
      );
      expect(handle).not.toBeNull();
      expect(handle!.className).not.toContain("cursor-grab");
    }
  });
});
