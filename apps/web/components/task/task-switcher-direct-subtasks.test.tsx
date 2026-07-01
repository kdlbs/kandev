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

function task(id: string, parentTaskId?: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", parentTaskId };
}

function groupedWithDirectSubtasks(childCount: number): GroupedSidebarList {
  const parent = task("Parent");
  const children = Array.from({ length: childCount }, (_, index) =>
    task(`Child ${index + 1}`, parent.id),
  );
  return {
    groups: [{ key: "__all__", label: "All", tasks: [parent] }],
    subTasksByParentId: new Map([[parent.id, children]]),
  };
}

describe("TaskSwitcher direct subtasks", () => {
  it("renders a parent with 17 direct subtasks", () => {
    const { container } = render(
      <Providers>
        <TaskSwitcher
          grouped={groupedWithDirectSubtasks(17)}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onToggleSubtasks={vi.fn()}
          collapsedSubtaskParentIds={[]}
        />
      </Providers>,
    );

    expect(container.querySelectorAll("[data-testid='sortable-task-block']")).toHaveLength(18);
    expect(screen.queryByText("Parent")).not.toBeNull();
    expect(screen.queryByText("Child 1")).not.toBeNull();
    expect(screen.queryByText("Child 17")).not.toBeNull();
  });
});
