import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";

afterEach(() => cleanup());

const REPOSITORY_TEST_ID = "sidebar-task-repository";
const REPOSITORY_SLUG = "kdlbs/kandev";
const REPO_TASK: TaskSwitcherItem = {
  id: "Task A",
  title: "Task A",
  state: "IN_PROGRESS",
  repositoryPath: REPOSITORY_SLUG,
};

function renderGroupedBy(
  groupKey: GroupedSidebarList["groupKey"],
  groups: GroupedSidebarList["groups"],
) {
  return render(
    <StateProvider>
      <ToastProvider>
        <TaskSwitcher
          grouped={{ groups, subTasksByParentId: new Map(), groupKey }}
          activeTaskId={null}
          selectedTaskId={null}
          unreadMessagesEnabled={false}
          unreadDotHideDelayMs={0}
          displayedSessionId={null}
          onSelectTask={vi.fn()}
        />
      </ToastProvider>
    </StateProvider>,
  );
}

describe("TaskSwitcher repository labels", () => {
  it("names each task's repository in a flat, ungrouped list", () => {
    renderGroupedBy("none", [{ key: "__all__", label: "All", tasks: [REPO_TASK] }]);
    expect(screen.getByTestId(REPOSITORY_TEST_ID).textContent).toBe(REPOSITORY_SLUG);
  });

  it("names each task's repository when grouped by a dimension other than repository", () => {
    renderGroupedBy("workflowStep", [{ key: "step-1", label: "In progress", tasks: [REPO_TASK] }]);
    expect(screen.getByTestId(REPOSITORY_TEST_ID).textContent).toBe(REPOSITORY_SLUG);
  });

  it("leaves the repository off the rows when the group header already names it", () => {
    renderGroupedBy("repository", [
      { key: REPOSITORY_SLUG, label: REPOSITORY_SLUG, tasks: [REPO_TASK] },
    ]);
    expect(screen.queryByTestId(REPOSITORY_TEST_ID)).toBeNull();
  });
});
