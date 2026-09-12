import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MobileActionConfirmation } from "./mobile-action-confirmation";

const viewport = vi.hoisted(() => ({ isMobile: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({ useResponsiveBreakpoint: () => viewport }));
afterEach(() => {
  cleanup();
  viewport.isMobile = true;
});

function Harness({
  onConfirm = vi.fn(),
  completionPolicy,
  targetKey = "A",
}: {
  onConfirm?: () => void | Promise<void>;
  completionPolicy?: "await-with-retry";
  targetKey?: string;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={trigger} onClick={() => setOpen(true)}>
        Remove A
      </button>
      <MobileActionConfirmation
        open={open}
        onOpenChange={setOpen}
        targetKey={targetKey}
        completionPolicy={completionPolicy}
        title="Delete item?"
        subject="A"
        description="This cannot be undone."
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onConfirm={onConfirm}
        focusReturnRef={trigger}
        fallback={open ? <div>Desktop confirmation</div> : null}
      />
    </>
  );
}

it("uses a standalone drawer and returns focus on cancellation", async () => {
  render(<Harness />);
  const trigger = screen.getByRole("button", { name: "Remove A" });
  fireEvent.click(trigger);
  expect(screen.getByRole("dialog", { name: "Delete item?" }).getAttribute("data-slot")).toBe(
    "drawer-content",
  );
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })),
  );
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});

it("closes before dispatch, prevents duplicate activation, and does not reopen on rejection", async () => {
  let reject!: (reason: Error) => void;
  let closedAtDispatch = false;
  const onConfirm = vi.fn(() => {
    closedAtDispatch = screen.queryByRole("group") === null;
    return new Promise<void>((_, fail) => {
      reject = fail;
    });
  });
  render(<Harness onConfirm={onConfirm} />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  const button = screen.getByRole("button", { name: "Delete" });
  act(() => {
    fireEvent.click(button);
    fireEvent.click(button);
  });
  await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
  expect(closedAtDispatch).toBe(true);
  await act(async () => {
    reject(new Error("delete failed"));
  });
  expect(screen.queryByRole("group")).toBeNull();
});

it("cancels crossing the phone boundary and leaves the non-phone fallback unchanged", () => {
  const onConfirm = vi.fn();
  const { rerender } = render(<Harness onConfirm={onConfirm} />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  expect(screen.getByRole("group")).toBeTruthy();
  viewport.isMobile = false;
  rerender(<Harness onConfirm={onConfirm} />);
  expect(screen.queryByRole("group")).toBeNull();
  expect(screen.queryByText("Desktop confirmation")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  expect(screen.getByText("Desktop confirmation")).toBeTruthy();
  expect(onConfirm).not.toHaveBeenCalled();
});

it("allows a newly opened request to supply its target after the idle state", () => {
  function SelectedTarget() {
    const [target, setTarget] = useState<string | null>(null);
    return (
      <>
        <button onClick={() => setTarget("A")}>Choose A</button>
        <MobileActionConfirmation
          open={target !== null}
          targetKey={target ?? ""}
          onOpenChange={(open) => {
            if (!open) setTarget(null);
          }}
          title="Delete item?"
          subject={target}
          cancelLabel="Cancel"
          confirmLabel="Delete"
          onConfirm={vi.fn()}
        />
      </>
    );
  }
  render(<SelectedTarget />);
  fireEvent.click(screen.getByRole("button", { name: "Choose A" }));
  expect(screen.getByRole("dialog", { name: "Delete item?" })).toBeTruthy();
});

it("keeps an opted-in request busy and retries rejection without duplicate dispatch", async () => {
  let reject!: () => void;
  const onConfirm = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<void>((_, fail) => {
          reject = fail;
        }),
    )
    .mockResolvedValue(undefined);
  render(<Harness onConfirm={onConfirm} completionPolicy="await-with-retry" />);
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  const button = screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement;
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("dialog", { name: "Delete item?" })).toBeTruthy();
  expect(button.disabled).toBe(true);
  await act(async () => reject());
  await waitFor(() => expect(button.disabled).toBe(false));
  fireEvent.click(button);
  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(2));
  expect(screen.queryByRole("group")).toBeNull();
});

it("does not let an old awaited rejection replace a newer target", async () => {
  let reject!: () => void;
  const onConfirm = vi.fn(
    () =>
      new Promise<void>((_, fail) => {
        reject = fail;
      }),
  );
  const { rerender } = render(
    <Harness onConfirm={onConfirm} completionPolicy="await-with-retry" />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
  rerender(<Harness onConfirm={onConfirm} completionPolicy="await-with-retry" targetKey="B" />);
  expect(screen.queryByRole("group")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Remove A" }));
  await act(async () => reject());
  expect(screen.getAllByRole("group")).toHaveLength(1);
  expect(onConfirm).toHaveBeenCalledTimes(1);
});
