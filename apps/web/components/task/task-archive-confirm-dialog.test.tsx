import { StrictMode, type ReactNode } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";

const mockGetSubtaskCount = vi.fn();

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
}));

import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";

function renderDialog(ui: ReactNode, confirmTaskArchive = true) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, confirmTaskArchive },
      }}
    >
      {ui}
    </StateProvider>,
  );
}

type SeedTask = { id: string; foregroundActivity?: "generating" | "background" | null };

// Seed the store's active kanban tasks so useTaskInFlight resolves the same
// foreground_activity aggregate the board shows — the guard reads live store
// state, not a prop.
function renderWithTasks(ui: ReactNode, tasks: SeedTask[], confirmTaskArchive = true) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, confirmTaskArchive },
        kanban: {
          workflowId: "wf-1",
          steps: [],
          tasks: tasks.map((t) => ({
            id: t.id,
            workflowStepId: "step-1",
            title: t.id,
            position: 0,
            foregroundActivity: t.foregroundActivity ?? undefined,
          })),
        },
      }}
    >
      {ui}
    </StateProvider>,
  );
}

const WARNING_TESTID = "still-working-warning";

function DisableArchiveConfirmationButton() {
  const settings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);

  return (
    <button
      type="button"
      onClick={() => setUserSettings({ ...settings, confirmTaskArchive: false })}
    >
      Disable archive confirmation
    </button>
  );
}

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
  mockGetSubtaskCount.mockResolvedValue({ count: 0 });
});

afterEach(cleanup);

describe("TaskArchiveConfirmDialog preference", () => {
  it("archives once without rendering a dialog when confirmation is disabled", async () => {
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();

    renderDialog(
      <StrictMode>
        <TaskArchiveConfirmDialog
          open
          onOpenChange={onOpenChange}
          taskTitle="My task"
          taskId="task-1"
          executorType="worktree"
          onConfirm={onConfirm}
        />
      </StrictMode>,
      false,
    );

    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith({ cascade: false }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("does not auto-archive an already-open dialog after a settings sync", () => {
    const onConfirm = vi.fn();

    renderDialog(
      <>
        <DisableArchiveConfirmationButton />
        <TaskArchiveConfirmDialog
          open
          onOpenChange={() => {}}
          taskTitle="My task"
          taskId="task-1"
          executorType="worktree"
          onConfirm={onConfirm}
        />
      </>,
    );

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    fireEvent.click(screen.getByText("Disable archive confirmation", { selector: "button" }));

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog")).toBeTruthy();
  });
});

describe("TaskArchiveConfirmDialog cleanup copy", () => {
  it("renders local-executor reassurance about untouched repo", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/directly in your repo/i)).toBeTruthy();
  });

  it("renders worktree-executor copy about worktree + branch removal", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/worktree and its branch will be deleted/i)).toBeTruthy();
  });

  it("warns about sandbox destruction for sprites executor", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="sprites"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Sprites cloud sandbox/i)).toBeTruthy();
    expect(screen.getByText(/uncommitted work/i)).toBeTruthy();
  });

  it("describes Docker container removal for local_docker", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local_docker"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Docker container/i)).toBeTruthy();
  });

  it("renders grouped copy for bulk archive", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={3}
        taskIds={["a", "b", "c"]}
        executorTypes={["sprites", "worktree", "worktree"]}
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/2 worktrees/i)).toBeTruthy();
    expect(screen.getByText(/1 Sprites sandbox/i)).toBeTruthy();
  });

  it("no longer renders the old hardcoded worktree line for non-worktree executors", () => {
    renderDialog(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(
      screen.queryByText(
        /This will delete the task's worktree and stop any running agent sessions/,
      ),
    ).toBeNull();
  });
});

describe("TaskArchiveConfirmDialog still-working guard", () => {
  it("warns when the task is generating", () => {
    renderWithTasks(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: "task-1", foregroundActivity: "generating" }],
    );
    expect(screen.getByTestId(WARNING_TESTID)).toBeTruthy();
    expect(screen.getByTestId(WARNING_TESTID).textContent).toMatch(/still working/i);
  });

  it("warns when spawned background work is still running", () => {
    renderWithTasks(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: "task-1", foregroundActivity: "background" }],
    );
    expect(screen.getByTestId(WARNING_TESTID)).toBeTruthy();
  });

  it("omits the warning for an idle task", () => {
    renderWithTasks(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId="task-1"
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: "task-1", foregroundActivity: null }],
    );
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.queryByTestId(WARNING_TESTID)).toBeNull();
  });

  it("warns for a bulk archive when any selected task is in-flight", () => {
    renderWithTasks(
      <TaskArchiveConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={2}
        taskIds={["a", "b"]}
        executorTypes={["worktree", "worktree"]}
        onConfirm={() => {}}
      />,
      [
        { id: "a", foregroundActivity: null },
        { id: "b", foregroundActivity: "generating" },
      ],
    );
    expect(screen.getByTestId(WARNING_TESTID)).toBeTruthy();
  });

  it("does not warn (or prompt) when archive confirmation is disabled, even mid-run", async () => {
    // Documented residual gap (operator decision q1_opt2): honoring the
    // confirmTaskArchive bypass means an in-flight task can be archived with no
    // dialog and therefore no warning. Delete has no such bypass.
    const onConfirm = vi.fn();
    renderWithTasks(
      <StrictMode>
        <TaskArchiveConfirmDialog
          open
          onOpenChange={() => {}}
          taskTitle="My task"
          taskId="task-1"
          executorType="worktree"
          onConfirm={onConfirm}
        />
      </StrictMode>,
      [{ id: "task-1", foregroundActivity: "generating" }],
      false,
    );
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith({ cascade: false }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.queryByTestId(WARNING_TESTID)).toBeNull();
  });
});
