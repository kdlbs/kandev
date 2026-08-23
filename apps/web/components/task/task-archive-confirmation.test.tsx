import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRef } from "react";
import { StateProvider } from "@/components/state-provider";

const getSubtaskCountMock = vi.hoisted(() => vi.fn());
const pointerState = vi.hoisted(() => ({ isFinePointer: false }));
const CONFIRM_TEST_ID = "archive-task-confirm";

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => getSubtaskCountMock(...args),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointerState,
}));

import { TaskArchiveConfirmation } from "./task-archive-confirmation";

afterEach(cleanup);

function ConfirmationHarness({
  onConfirm,
  onOpenChange,
}: {
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button" data-testid="archive-anchor">
        Archive source
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
      />
    </>
  );
}

function renderConfirmation(onConfirm = vi.fn(), onOpenChange = vi.fn()) {
  return render(
    <StateProvider>
      <ConfirmationHarness onConfirm={onConfirm} onOpenChange={onOpenChange} />
    </StateProvider>,
  );
}

describe("TaskArchiveConfirmation classification", () => {
  beforeEach(() => {
    pointerState.isFinePointer = false;
    getSubtaskCountMock.mockReset();
  });

  it("does not expose an archive action while descendant classification is pending", () => {
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));

    renderConfirmation();

    expect(screen.queryByTestId(CONFIRM_TEST_ID)).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("shows a disabled anchored loading surface while desktop classification is pending", () => {
    pointerState.isFinePointer = true;
    getSubtaskCountMock.mockReturnValue(new Promise(() => undefined));
    const onOpenChange = vi.fn();

    renderConfirmation(vi.fn(), onOpenChange);

    expect(screen.getByTestId("task-archive-confirm-popover")).toBeTruthy();
    expect(screen.getByText("Loading…")).toBeTruthy();
    expect(screen.getByTestId(CONFIRM_TEST_ID).hasAttribute("disabled")).toBe(true);
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("uses touch-sized local actions after a resolved zero-descendant result", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });

    renderConfirmation();

    const confirmation = await screen.findByTestId("task-archive-inline-confirmation");
    expect(confirmation).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId(CONFIRM_TEST_ID).className).toContain("h-11");
  });

  it("keeps descendants on the cascade dialog branch", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 2 });

    renderConfirmation();

    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toBeTruthy();
    expect(screen.getByTestId("archive-cascade-checkbox")).toBeTruthy();
    expect(screen.queryByTestId("task-archive-inline-confirmation")).toBeNull();
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
      closedBeforeConfirm = screen.queryByTestId("task-archive-inline-confirmation") === null;
    });

    renderConfirmation(onConfirm);
    fireEvent.click(await screen.findByTestId(CONFIRM_TEST_ID));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    expect(closedBeforeConfirm).toBe(true);
  });
});
