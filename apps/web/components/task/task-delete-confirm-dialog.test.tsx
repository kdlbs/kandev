import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";

const mockGetSubtaskCount = vi.fn();
const mockGetTaskDeletePreflight = vi.fn();

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => mockGetSubtaskCount(...args),
  getTaskDeletePreflight: (...args: unknown[]) => mockGetTaskDeletePreflight(...args),
}));

import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";
import { expectCompactWarning } from "./task-confirm-dialog.test-helpers";

type SeedTask = { id: string; foregroundActivity?: "generating" | "background" | null };

// The dialog reads live foreground_activity from the store via useTaskInFlight,
// so every render needs a StateProvider. `tasks` seeds the active kanban tasks
// the guard resolves.
function renderDialog(ui: ReactNode, tasks: SeedTask[] = []) {
  return render(
    <StateProvider
      initialState={{
        kanban: {
          workflowId: "wf-1",
          steps: [],
          tasks: tasks.map((t) => ({
            id: t.id,
            workflowId: "wf-1",
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
const DISCARD_CHECKBOX_TESTID = "delete-discard-worktree-checkbox";
const CASCADE_CHECKBOX_TESTID = "delete-cascade-checkbox";
const TASK_ID = "task-1";

beforeEach(() => {
  mockGetSubtaskCount.mockReset();
  mockGetTaskDeletePreflight.mockReset();
  mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: false });
});

afterEach(cleanup);

describe("TaskDeleteConfirmDialog", () => {
  it("contains long confirmation content in a scrolling body with touch-safe actions", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="A task with a title that needs to wrap inside a phone confirmation surface"
        taskId={TASK_ID}
        executorType="sprites"
        isInFlight
        confirmTestId="confirm"
        onConfirm={() => {}}
      />,
    );

    const dialog = screen.getByRole("alertdialog");
    expect(dialog.className).toContain("max-h-[calc(100dvh-2rem)]");
    expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(dialog.className).toContain("overflow-hidden");
    expect(screen.getByTestId("task-confirmation-body").className).toContain("min-h-0");
    expect(screen.getByTestId("task-confirmation-body").className).toContain("space-y-3");
    expect(screen.getByTestId("task-confirmation-body").className).toContain("overflow-y-auto");
    expect(screen.getByTestId("confirm").className).toContain("min-h-[44px]");
    expect(screen.getByTestId("confirm").className).toContain("w-full");
    expect(screen.getByTestId("confirm").getAttribute("data-variant")).toBe("destructive");
  });

  it("hides the cascade checkbox when the task has no subtasks", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    const onConfirm = vi.fn();
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    await waitFor(() => expect(mockGetSubtaskCount).toHaveBeenCalledWith(TASK_ID));
    expect(screen.queryByTestId(CASCADE_CHECKBOX_TESTID)).toBeNull();

    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false, discardWorktreeChanges: false });
  });

  it("shows the cascade checkbox when the task has subtasks; defaults to unchecked", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 3 });
    const onConfirm = vi.fn();
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    await screen.findByTestId(CASCADE_CHECKBOX_TESTID);
    expect(screen.getByText(/Also delete 3 subtasks/i)).toBeTruthy();

    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false, discardWorktreeChanges: false });
  });

  it("propagates cascade=true when the user ticks the checkbox", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 2 });
    const onConfirm = vi.fn();
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );
    const checkbox = await screen.findByTestId(CASCADE_CHECKBOX_TESTID);
    fireEvent.click(checkbox);
    await waitFor(() => expect(mockGetTaskDeletePreflight).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: true, discardWorktreeChanges: false });
  });

  it("sums subtask counts across taskIds for bulk delete", async () => {
    mockGetSubtaskCount.mockImplementation((id: string) =>
      Promise.resolve({ count: id === "a" ? 2 : 5 }),
    );
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={2}
        taskIds={["a", "b"]}
        onConfirm={() => {}}
      />,
    );
    await screen.findByText(/Also delete 7 subtasks/i);
  });
});

describe("TaskDeleteConfirmDialog discard consent", () => {
  it("hides discard consent for a clean worktree and confirms without consent", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: false });
    const onConfirm = vi.fn();
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );

    await waitFor(() => expect(mockGetTaskDeletePreflight).toHaveBeenCalledWith([TASK_ID], false));
    expect(screen.queryByTestId(DISCARD_CHECKBOX_TESTID)).toBeNull();
    fireEvent.click(screen.getByTestId("confirm"));
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false, discardWorktreeChanges: false });
  });

  it("requires explicit discard consent for worktree cleanup", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: true });
    const onConfirm = vi.fn();
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={onConfirm}
        confirmTestId="confirm"
      />,
    );

    const discard = await screen.findByTestId(DISCARD_CHECKBOX_TESTID);
    const confirm = screen.getByTestId("confirm") as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    fireEvent.click(confirm);
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(discard);
    expect(confirm.disabled).toBe(false);
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledWith({ cascade: false, discardWorktreeChanges: true });
  });

  it("shows discard consent when a cascade can include child worktrees", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 1 });
    mockGetTaskDeletePreflight.mockResolvedValue({ requires_discard_consent: true });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="Parent task"
        taskId={TASK_ID}
        executorType="local"
        onConfirm={() => {}}
      />,
    );

    expect(await screen.findByTestId(DISCARD_CHECKBOX_TESTID)).toBeTruthy();
  });
});

describe("TaskDeleteConfirmDialog discard consent state", () => {
  it("hides the discard option and disables Delete while inspection is pending", async () => {
    let resolvePreflight: (value: { requires_discard_consent: boolean }) => void = () => {};
    mockGetTaskDeletePreflight.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolvePreflight = resolve;
        }),
    );
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
        confirmTestId="confirm"
      />,
    );

    expect(await screen.findByTestId("delete-preflight-loading")).toBeTruthy();
    expect(screen.queryByTestId(DISCARD_CHECKBOX_TESTID)).toBeNull();
    expect((screen.getByTestId("confirm") as HTMLButtonElement).disabled).toBe(true);
    resolvePreflight({ requires_discard_consent: false });
    await waitFor(() =>
      expect((screen.getByTestId("confirm") as HTMLButtonElement).disabled).toBe(false),
    );
  });

  it("shows a retry action and keeps Delete disabled after inspection errors", async () => {
    mockGetTaskDeletePreflight.mockRejectedValueOnce(new Error("inspection failed"));
    mockGetTaskDeletePreflight.mockResolvedValueOnce({ requires_discard_consent: true });
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
        confirmTestId="confirm"
      />,
    );

    const error = await screen.findByTestId("delete-preflight-error");
    expect(error).toBeTruthy();
    expect(screen.queryByTestId(DISCARD_CHECKBOX_TESTID)).toBeNull();
    expect((screen.getByTestId("confirm") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByTestId(DISCARD_CHECKBOX_TESTID)).toBeTruthy();
    expect((screen.getByTestId("confirm") as HTMLButtonElement).disabled).toBe(true);
  });

  it("invalidates checked consent when the delete scope changes", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 1 });
    mockGetTaskDeletePreflight.mockImplementation((_ids: string[], cascade: boolean) =>
      Promise.resolve({ requires_discard_consent: !cascade }),
    );
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="Parent task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
        confirmTestId="confirm"
      />,
    );

    const discard = await screen.findByTestId(DISCARD_CHECKBOX_TESTID);
    fireEvent.click(discard);
    expect(discard.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(screen.getByTestId(CASCADE_CHECKBOX_TESTID));
    await waitFor(() => expect(mockGetTaskDeletePreflight).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId(DISCARD_CHECKBOX_TESTID)).toBeNull();
    fireEvent.click(screen.getByTestId(CASCADE_CHECKBOX_TESTID));
    const freshDiscard = await screen.findByTestId(DISCARD_CHECKBOX_TESTID);
    expect(freshDiscard.getAttribute("aria-checked")).toBe("false");
  });
});

describe("TaskDeleteConfirmDialog executor cleanup copy", () => {
  it("states the named delete outcome directly and separates cleanup effects", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
    );

    expect(screen.getByTestId("task-confirmation-outcome").textContent).toMatch(
      /Delete [“"]?My task[”"]?\. This action cannot be undone\./i,
    );
    expect(screen.getByTestId("task-cleanup-effects").tagName).toBe("UL");
    expect(screen.getByTestId("task-cleanup-effects").querySelectorAll("li")).toHaveLength(2);
    expect(screen.getByTestId("task-cleanup-notes").tagName).toBe("DIV");
    expect(screen.queryByText(/Are you sure/i)).toBeNull();
  });

  it("local reassures repo is untouched", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="local"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/directly in your repo/i)).toBeTruthy();
    expect(screen.getByText(/not touched/i)).toBeTruthy();
  });

  it("worktree describes worktree+branch removal", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/worktree and its branch will be deleted/i)).toBeTruthy();
  });

  it("groups bulk delete copy by executor type", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        isBulkOperation
        count={3}
        taskIds={["a", "b", "c"]}
        executorTypes={["worktree", "worktree", "local"]}
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/2 worktrees/i)).toBeTruthy();
    expect(screen.getByText(/1 local task/i)).toBeTruthy();
  });

  it("falls back to a generic message when no executorType is provided", async () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        onConfirm={() => {}}
      />,
    );
    expect(screen.getByText(/Any running agent sessions will be stopped/i)).toBeTruthy();
    expect(screen.queryByTestId(DISCARD_CHECKBOX_TESTID)).toBeNull();
  });
});

describe("TaskDeleteConfirmDialog still-working guard", () => {
  it("keeps the shared in-flight warning visually subordinate", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: TASK_ID, foregroundActivity: "generating" }],
    );

    expectCompactWarning(screen.getByTestId(WARNING_TESTID));
  });

  it("warns when the task is generating", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: TASK_ID, foregroundActivity: "generating" }],
    );
    expect(screen.getByTestId(WARNING_TESTID)).toBeTruthy();
    expect(screen.getByTestId(WARNING_TESTID).textContent).toMatch(/still working/i);
  });

  it("warns when spawned background work is still running", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: TASK_ID, foregroundActivity: "background" }],
    );
    expect(screen.getByTestId(WARNING_TESTID)).toBeTruthy();
  });

  it("omits the warning for an idle task", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
        open
        onOpenChange={() => {}}
        taskTitle="My task"
        taskId={TASK_ID}
        executorType="worktree"
        onConfirm={() => {}}
      />,
      [{ id: TASK_ID, foregroundActivity: null }],
    );
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.queryByTestId(WARNING_TESTID)).toBeNull();
  });

  it("warns for a bulk delete when any selected task is in-flight", () => {
    mockGetSubtaskCount.mockResolvedValue({ count: 0 });
    renderDialog(
      <TaskDeleteConfirmDialog
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
});
