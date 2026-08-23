import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRef } from "react";
import { StateProvider } from "@/components/state-provider";

const getSubtaskCountMock = vi.hoisted(() => vi.fn());
const pointerState = vi.hoisted(() => ({ isFinePointer: false }));

vi.mock("@/lib/api", () => ({
  getSubtaskCount: (...args: unknown[]) => getSubtaskCountMock(...args),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointerState,
}));

import { TaskArchiveConfirmation } from "./task-archive-confirmation";

afterEach(cleanup);

function ConfirmationHarness({ onConfirm }: { onConfirm: () => void }) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} type="button" data-testid="archive-anchor">
        Archive source
      </button>
      <TaskArchiveConfirmation
        open
        onOpenChange={vi.fn()}
        anchorRef={anchorRef}
        taskId="task-1"
        taskTitle="Task One"
        executorType="worktree"
        onConfirm={onConfirm}
        confirmTestId="archive-task-confirm"
      />
    </>
  );
}

function renderConfirmation(onConfirm = vi.fn()) {
  return render(
    <StateProvider>
      <ConfirmationHarness onConfirm={onConfirm} />
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

    expect(screen.queryByTestId("archive-task-confirm")).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("uses touch-sized local actions after a resolved zero-descendant result", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });

    renderConfirmation();

    const confirmation = await screen.findByTestId("task-archive-inline-confirmation");
    expect(confirmation).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId("archive-task-confirm").className).toContain("h-11");
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
    fireEvent.click(await screen.findByTestId("archive-task-confirm"));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    expect(closedBeforeConfirm).toBe(true);
  });
});
