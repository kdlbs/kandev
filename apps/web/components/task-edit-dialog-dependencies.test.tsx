import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { TaskEditDialogDependenciesState } from "@/hooks/domains/task/use-task-edit-dialog-dependencies";
import { ApiError } from "@/lib/api/client";
import { TaskEditDialogDependencies } from "./task-edit-dialog-dependencies";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number; cycle?: string }) =>
      ({
        "task:dependsOn": "Depends on",
        "task:noDependency": "No dependency",
        "task:searchTasks": "Search tasks...",
        "task:noTasksFound": "No tasks found.",
        "tasks:loadingTasks": "Loading tasks...",
        "task:dependencyInfoLabel": "About task dependencies",
        "task:dependencyInfo": "This task waits until every selected task completes successfully.",
        "task:failedToLoadDependencies": "Failed to load dependencies.",
        "task:failedToLoadDependencyCandidates": "Failed to load dependency candidates.",
        "task:dependencyUpdateFailed": "Failed to update dependencies.",
        "task:dependencyCycleError": `Dependency cycle: ${options?.cycle ?? ""}`,
        "task:retry": "Retry",
        "task:dependencyCount_one": `${options?.count ?? 0} dependency`,
        "task:dependencyCount_other": `${options?.count ?? 0} dependencies`,
      })[key] ?? key,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ kanban: { tasks: [] }, kanbanMulti: { snapshots: {} } }),
}));

function state(
  overrides: Partial<TaskEditDialogDependenciesState> = {},
): TaskEditDialogDependenciesState {
  return {
    taskId: "task-1",
    confirmedIds: ["task-2"],
    draftIds: ["task-2"],
    setDraftIds: vi.fn(),
    selectedTitles: { "task-2": "Predecessor" },
    candidates: [
      { id: "task-1", title: "Edited task" },
      { id: "task-2", title: "Predecessor" },
      { id: "task-3", title: "Another task" },
      { id: "task-archived", title: "Archived task", isArchived: true },
    ],
    candidatesLoading: false,
    query: "",
    setQuery: vi.fn(),
    loading: false,
    loadError: null,
    candidateError: null,
    saveError: null,
    error: null,
    submitError: null,
    ready: true,
    isDirty: false,
    save: vi.fn(),
    retry: vi.fn(),
    ...overrides,
  };
}

function renderDependencies(value: TaskEditDialogDependenciesState) {
  return render(
    <TooltipProvider>
      <TaskEditDialogDependencies state={value} />
    </TooltipProvider>,
  );
}

afterEach(cleanup);

describe("TaskEditDialogDependencies", () => {
  it("uses the current predecessor set and excludes the edited and archived tasks", () => {
    renderDependencies(state());

    expect(screen.getByTestId("task-edit-dependencies")).toBeTruthy();
    expect(screen.getByText("Depends on")).toBeTruthy();
    expect(screen.getByTestId("task-create-dependencies-trigger").textContent).toContain(
      "Predecessor",
    );

    fireEvent.click(screen.getByTestId("task-create-dependencies-trigger"));
    const picker = screen.getByTestId("task-create-dependencies-popover");
    expect(within(picker).getByTestId("task-create-dependency-option-task-2")).toBeTruthy();
    expect(within(picker).getByTestId("task-create-dependency-option-task-3")).toBeTruthy();
    expect(within(picker).queryByText("Edited task")).toBeNull();
    expect(within(picker).queryByText("Archived task")).toBeNull();
  });

  it("keeps the editor open and names the cycle when saving fails", () => {
    const cycleError = new ApiError("would create a dependency cycle", 409, {
      cycle: ["task-1", "task-2", "task-1"],
    });
    renderDependencies(
      state({
        saveError: cycleError,
        submitError: {
          dependencyUpdate: true,
          cause: cycleError,
        },
        error: cycleError,
      }),
    );

    expect(screen.getByTestId("task-edit-dependencies")).toBeTruthy();
    expect(screen.getByTestId("task-edit-dependencies-error").textContent).toContain(
      "Dependency cycle: task-1 -> task-2 -> task-1",
    );
  });
});
