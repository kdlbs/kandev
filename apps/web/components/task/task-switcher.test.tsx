import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import { createTaskLinkSelectAction } from "./task-switcher-context-menu";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";

afterEach(() => cleanup());

function Providers({ children }: { children: React.ReactNode }) {
  return (
    <StateProvider>
      <ToastProvider>{children}</ToastProvider>
    </StateProvider>
  );
}

function item(id: string, parentTaskId?: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", parentTaskId };
}

// root → child → grandchild (depth 2)
const ROOT = item("Root");
const CHILD = item("Child", "Root");
const GRANDCHILD = item("Grandchild", "Child");
const TASK_A = item("Task A");
const TASK_B = item("Task B");

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

describe("TaskSwitcher — bulk pin menu", () => {
  it("offers bulk unpin when every selected root task is pinned", () => {
    render(
      <Providers>
        <TaskSwitcher
          grouped={{
            groups: [{ key: "__all__", label: "All", tasks: [TASK_A, TASK_B] }],
            subTasksByParentId: new Map(),
          }}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onBulkPin={vi.fn()}
          pinnedTaskIds={[TASK_A.id, TASK_B.id]}
          selectedTaskIds={new Set([TASK_A.id, TASK_B.id])}
        />
      </Providers>,
    );

    fireEvent.contextMenu(screen.getByText(TASK_A.title));

    expect(screen.getByText("Unpin 2 tasks")).toBeTruthy();
  });
});

describe("TaskSwitcher — detach menu", () => {
  it("offers detach for a single subtask and invokes the action", () => {
    const onDetachTask = vi.fn();
    render(
      <Providers>
        <TaskSwitcher
          grouped={grouped()}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onDetachTask={onDetachTask}
        />
      </Providers>,
    );

    fireEvent.contextMenu(screen.getByText(CHILD.title));
    fireEvent.click(screen.getByRole("menuitem", { name: "Detach from parent" }));

    expect(onDetachTask).toHaveBeenCalledWith(CHILD.id);
  });

  it("omits detach for root and multi-selection menus", () => {
    const { rerender } = render(
      <Providers>
        <TaskSwitcher
          grouped={grouped()}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onDetachTask={vi.fn()}
        />
      </Providers>,
    );

    fireEvent.contextMenu(screen.getByText(ROOT.title));
    expect(screen.queryByRole("menuitem", { name: "Detach from parent" })).toBeNull();

    rerender(
      <Providers>
        <TaskSwitcher
          grouped={grouped()}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onDetachTask={vi.fn()}
          selectedTaskIds={new Set([CHILD.id, GRANDCHILD.id])}
        />
      </Providers>,
    );
    fireEvent.contextMenu(screen.getByText(CHILD.title));
    expect(screen.queryByRole("menuitem", { name: "Detach from parent" })).toBeNull();
  });
});

describe("TaskSwitcher — external issue link menu", () => {
  it("offers configured external issue providers from the task context menu", async () => {
    const onLinkMergeRequest = vi.fn();
    render(
      <Providers>
        <TaskSwitcher
          grouped={{
            groups: [{ key: "__all__", label: "All", tasks: [TASK_A] }],
            subTasksByParentId: new Map(),
          }}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onLinkPullRequest={vi.fn()}
          onLinkIssue={vi.fn()}
          onLinkMergeRequest={onLinkMergeRequest}
          onLinkJiraTicket={vi.fn()}
          onLinkLinearIssue={vi.fn()}
          onLinkSentryIssue={vi.fn()}
        />
      </Providers>,
    );

    fireEvent.contextMenu(screen.getByText(TASK_A.title));
    const linkTrigger = screen.getByText("Link");
    fireEvent.focus(linkTrigger);
    fireEvent.keyDown(linkTrigger, { key: "ArrowRight" });

    await waitFor(() => {
      expect(screen.getByText("Jira Ticket")).toBeTruthy();
      expect(screen.getByText("Linear Issue")).toBeTruthy();
      expect(screen.getByText("Sentry Issue")).toBeTruthy();
      expect(screen.getByText("GitLab Merge Request")).toBeTruthy();
    });
  });

  it("targets the selected task when linking a GitLab merge request", () => {
    const archivedTask: TaskSwitcherItem = {
      id: "archived-task",
      title: "Archived task title",
      isArchived: true,
    };
    const closeMenu = vi.fn();
    const onLinkMergeRequest = vi.fn();

    createTaskLinkSelectAction(archivedTask, onLinkMergeRequest, closeMenu)?.();

    expect(onLinkMergeRequest).toHaveBeenCalledWith(archivedTask.id, archivedTask.title);
    expect(closeMenu).toHaveBeenCalledOnce();
  });
});
