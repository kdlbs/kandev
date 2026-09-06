import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRef } from "react";
import { StateProvider } from "@/components/state-provider";

const getSubtaskCountMock = vi.hoisted(() => vi.fn());
const pointerState = vi.hoisted(() => ({ isFinePointer: false }));
const CONFIRM_TEST_ID = "archive-task-confirm";
const INLINE_CONFIRMATION_TEST_ID = "task-archive-inline-confirmation";
const CLEANUP_EFFECTS_TEST_ID = "task-cleanup-effects";
const CLEANUP_NOTES_TEST_ID = "task-cleanup-notes";

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => getSubtaskCountMock(...args),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointerState,
}));

import { TaskArchiveConfirmation } from "./task-archive-confirmation";

afterEach(cleanup);
beforeEach(() => {
  pointerState.isFinePointer = false;
  getSubtaskCountMock.mockReset();
});

function ConfirmationHarness({
  onConfirm,
  onOpenChange,
  forceDialog = false,
}: {
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
  forceDialog?: boolean;
}) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button" data-testid="archive-anchor">
        Archive source
      </button>
      <button type="button" data-testid="outside-action">
        Outside action
      </button>
      <TaskArchiveConfirmation
        open
        onOpenChange={onOpenChange}
        anchorRef={anchorRef}
        taskId="task-1"
        taskTitle="Task One"
        executorType="worktree"
        onConfirm={onConfirm}
        confirmTestId={CONFIRM_TEST_ID}
        forceDialog={forceDialog}
      />
    </>
  );
}

function renderConfirmation(onConfirm = vi.fn(), onOpenChange = vi.fn(), forceDialog = false) {
  return render(
    <StateProvider>
      <ConfirmationHarness
        onConfirm={onConfirm}
        onOpenChange={onOpenChange}
        forceDialog={forceDialog}
      />
    </StateProvider>,
  );
}

function deferredSubtaskCount() {
  let resolve: (result: { count: number }) => void = () => {};
  const promise = new Promise<{ count: number }>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("TaskArchiveConfirmation pending dismissal", () => {
  it("dismisses the hidden desktop request on Escape and restores trigger focus", async () => {
    pointerState.isFinePointer = true;
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));
    const onOpenChange = vi.fn();

    renderConfirmation(vi.fn(), onOpenChange);
    await waitFor(() => expect(getSubtaskCountMock).toHaveBeenCalledWith("task-1"));

    const outsideAction = screen.getByTestId("outside-action");
    outsideAction.focus();
    fireEvent.keyDown(outsideAction, { key: "Escape" });

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(document.activeElement).toBe(screen.getByTestId("archive-anchor"));
  });

  it("dismisses the hidden desktop request on outside pointer intent", async () => {
    pointerState.isFinePointer = true;
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));
    const onOpenChange = vi.fn();

    renderConfirmation(vi.fn(), onOpenChange);
    await waitFor(() => expect(getSubtaskCountMock).toHaveBeenCalledWith("task-1"));

    fireEvent.pointerDown(screen.getByTestId("outside-action"));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("TaskArchiveConfirmation classification", () => {
  it("does not expose an archive action while descendant classification is pending", () => {
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));

    renderConfirmation();

    expect(screen.queryByTestId(CONFIRM_TEST_ID)).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  // @covers AC-TASKS-CONFIRMATION-SURFACE-002.4
  it("waits for desktop classification before showing only the cascade dialog", async () => {
    pointerState.isFinePointer = true;
    const deferredCount = deferredSubtaskCount();
    getSubtaskCountMock.mockReturnValue(deferredCount.promise);

    renderConfirmation();
    await waitFor(() => expect(getSubtaskCountMock).toHaveBeenCalledWith("task-1"));

    expect(screen.queryByTestId("task-archive-confirm-popover")).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.queryByTestId(CONFIRM_TEST_ID)).toBeNull();

    await act(async () => deferredCount.resolve({ count: 2 }));

    expect(await screen.findByRole("alertdialog")).toBeTruthy();
    expect(screen.getByTestId("archive-cascade-checkbox")).toBeTruthy();
    expect(screen.queryByTestId("task-archive-confirm-popover")).toBeNull();
  });

  it("uses touch-sized local actions after a resolved zero-descendant result", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });

    renderConfirmation();

    const confirmation = await screen.findByTestId(INLINE_CONFIRMATION_TEST_ID);
    expect(confirmation).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId(CONFIRM_TEST_ID).className).toContain("h-11");
  });

  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.8
  it("uses the contained dialog when a coarse-pointer caller forces it", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();

    renderConfirmation(onConfirm, onOpenChange, true);

    const dialog = await screen.findByRole("alertdialog", { name: /Archive task/ });
    expect(dialog).toBeTruthy();
    expect(screen.queryByTestId(INLINE_CONFIRMATION_TEST_ID)).toBeNull();
    expect(screen.getByTestId("task-confirmation-outcome").textContent).toContain("Task One");

    const archive = screen.getByTestId(CONFIRM_TEST_ID);
    expect(archive.className).toContain("min-h-11");
    expect(archive.className).toContain("w-full");
    expect(archive.getAttribute("data-variant")).toBe("default");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);

    fireEvent.click(archive);
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith({ cascade: false }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps the forced dialog archive action disabled while classification is pending", async () => {
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));

    renderConfirmation(vi.fn(), vi.fn(), true);

    const dialog = await screen.findByRole("alertdialog", { name: /Archive task/ });
    expect(dialog).toBeTruthy();
    expect(screen.queryByTestId(INLINE_CONFIRMATION_TEST_ID)).toBeNull();
    expect(screen.getByTestId(CONFIRM_TEST_ID).hasAttribute("disabled")).toBe(true);
  });

  it("uses the same semantic cleanup effect list and supporting notes as the dialog", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });

    renderConfirmation();

    await screen.findByTestId(INLINE_CONFIRMATION_TEST_ID);
    expect(screen.getByTestId(CLEANUP_EFFECTS_TEST_ID).getAttribute("role")).toBe("list");
    expect(
      screen.getByTestId(CLEANUP_EFFECTS_TEST_ID).querySelectorAll('[role="listitem"]'),
    ).toHaveLength(2);
    expect(screen.getByTestId(CLEANUP_NOTES_TEST_ID).tagName).toBe("SPAN");
    expect(screen.queryByText(/Are you sure/i)).toBeNull();
  });

  it("keeps descendants on the cascade dialog branch", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 2 });

    renderConfirmation();

    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toBeTruthy();
    expect(screen.getByTestId("archive-cascade-checkbox")).toBeTruthy();
    expect(screen.queryByTestId(INLINE_CONFIRMATION_TEST_ID)).toBeNull();
  });

  it("keeps an unknown classification on the safe dialog branch", async () => {
    getSubtaskCountMock.mockRejectedValue(new Error("classification unavailable"));

    renderConfirmation();

    expect(await screen.findByRole("alertdialog")).toBeTruthy();
    expect(screen.queryByTestId("archive-cascade-checkbox")).toBeNull();
  });

  it("closes the local surface before invoking the archive callback", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });
    let closedBeforeConfirm = false;
    const onConfirm = vi.fn(() => {
      closedBeforeConfirm = screen.queryByTestId(INLINE_CONFIRMATION_TEST_ID) === null;
    });

    renderConfirmation(onConfirm);
    fireEvent.click(await screen.findByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    expect(closedBeforeConfirm).toBe(true);
  });
});

describe("TaskArchiveConfirmation cleanup copy", () => {
  it("uses the same semantic cleanup effect list in the fine-pointer popover", async () => {
    pointerState.isFinePointer = true;
    getSubtaskCountMock.mockResolvedValue({ count: 0 });

    renderConfirmation();

    await screen.findByTestId("task-archive-confirm-popover");
    expect(screen.getByTestId(CLEANUP_EFFECTS_TEST_ID).getAttribute("role")).toBe("list");
    expect(
      screen.getByTestId(CLEANUP_EFFECTS_TEST_ID).querySelectorAll('[role="listitem"]'),
    ).toHaveLength(2);
    expect(screen.getByTestId(CLEANUP_NOTES_TEST_ID).tagName).toBe("SPAN");
  });
});
