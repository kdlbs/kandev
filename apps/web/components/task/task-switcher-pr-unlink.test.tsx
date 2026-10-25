import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";

const API_UNLINK_LABEL = "Remove acme/api #42 from task";
const WEB_UNLINK_LABEL = "Remove acme/web #42 from task";

afterEach(() => cleanup());

function linkedPR(id: string, repo: string, taskId: string): TaskPR {
  return {
    id,
    workspace_id: "workspace-1",
    task_id: taskId,
    repository_id: `repository-${repo}`,
    owner: "acme",
    repo,
    pr_number: 42,
    pr_url: `https://github.com/acme/${repo}/pull/42`,
    pr_title: `${repo} PR`,
    head_branch: `feature/${repo}`,
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
  };
}

function task(id: string, withPR = false, editable = true): TaskSwitcherItem {
  return {
    id,
    title: id,
    state: "IN_PROGRESS",
    ...(editable ? { workflowId: "workflow-1", workflowStepId: "step-1" } : {}),
    ...(withPR ? { prInfo: { number: 42, state: "open" } } : {}),
  };
}

function renderTaskMenu(
  tasks: TaskSwitcherItem[],
  prs: TaskPR[],
  options: { selectedTaskIds?: Set<string>; rowActions?: boolean } = {},
) {
  const byTaskId = prs.reduce<Record<string, TaskPR[]>>((result, pr) => {
    result[pr.task_id] = [...(result[pr.task_id] ?? []), pr];
    return result;
  }, {});
  const initialState: Partial<AppState> = {
    workspaces: { items: [], activeId: "workspace-1" },
    taskPRs: { byTaskId, workspaceId: "workspace-1", workspaceContextGeneration: 0 },
  };
  return render(
    <TooltipProvider>
      <StateProvider initialState={initialState}>
        <ToastProvider>
          <TaskSwitcher
            grouped={{
              groups: [{ key: "__all__", label: "All", tasks }],
              subTasksByParentId: new Map(),
            }}
            activeTaskId={null}
            selectedTaskId={null}
            onSelectTask={vi.fn()}
            onEditTask={options.rowActions ? vi.fn() : undefined}
            onRenameTask={options.rowActions ? vi.fn() : undefined}
            selectedTaskIds={options.selectedTaskIds}
          />
        </ToastProvider>
      </StateProvider>
    </TooltipProvider>,
  );
}

describe("TaskSwitcher PR unlink actions", () => {
  it("lists each repository association under Edit while keeping row actions at root", async () => {
    const linkedTask = task("PR-linked task", true);
    renderTaskMenu(
      [linkedTask],
      [
        linkedPR("association-web-42", "web", linkedTask.id),
        linkedPR("association-api-42", "api", linkedTask.id),
      ],
      { rowActions: true },
    );

    fireEvent.contextMenu(screen.getByText(linkedTask.title));
    const edit = await screen.findByTestId("task-row-edit-submenu");
    fireEvent.pointerMove(edit, { pointerType: "mouse" });

    expect(await screen.findByRole("menuitem", { name: "Edit task" })).not.toBeNull();
    expect(await screen.findByRole("menuitem", { name: API_UNLINK_LABEL })).not.toBeNull();
    expect(await screen.findByRole("menuitem", { name: WEB_UNLINK_LABEL })).not.toBeNull();
    expect(screen.getByRole("menuitem", { name: "Rename" })).not.toBeNull();
    expect(screen.getByRole("menuitem", { name: "Duplicate" })).not.toBeNull();
  });

  it("hides single-task unlink when the row opens a reduced multi-selection menu", () => {
    const linkedTask = task("Selected PR task", true, false);
    const otherTask = task("Other selected task", false, false);
    renderTaskMenu(
      [linkedTask, otherTask],
      [linkedPR("association-api-42", "api", linkedTask.id)],
      { selectedTaskIds: new Set([linkedTask.id, otherTask.id]) },
    );

    fireEvent.contextMenu(screen.getByText(linkedTask.title));

    expect(screen.queryByTestId("task-row-edit-submenu")).toBeNull();
    expect(screen.queryByRole("menuitem", { name: API_UNLINK_LABEL })).toBeNull();
  });
});
