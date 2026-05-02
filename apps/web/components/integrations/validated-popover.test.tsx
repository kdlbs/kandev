import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, act, waitFor, cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
});

// Stub the Radix-backed Popover/Tooltip primitives so children render inline
// (no portal, no measurement gymnastics) and the popover's open/close is
// driven by ValidatedPopover's own onOpenChange callback that we surface via
// a parent-controlled close button.
vi.mock("@kandev/ui/popover", () => {
  return {
    Popover: ({
      open,
      onOpenChange,
      children,
    }: {
      open: boolean;
      onOpenChange: (next: boolean) => void;
      children: React.ReactNode;
    }) => (
      <div data-testid="popover" data-open={open}>
        {children}
        <button data-testid="popover-close" type="button" onClick={() => onOpenChange(false)}>
          close
        </button>
      </div>
    ),
    PopoverTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    PopoverContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  };
});

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { ValidatedPopover } from "./validated-popover";

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function renderHarness(overrides?: { fetch?: (key: string) => Promise<string> }) {
  const onSuccess = vi.fn();
  const result = render(
    <ValidatedPopover
      triggerStyle="ghost-icon"
      triggerIcon={<span>icon</span>}
      triggerAriaLabel="trigger"
      tooltip="tip"
      headline="Headline"
      placeholder="paste here"
      extractKey={(raw) => (raw.trim() ? raw.trim() : null)}
      validationHint="hint"
      fetch={overrides?.fetch ?? (async (k: string) => k)}
      onSuccess={onSuccess}
      submitLabel="Submit"
      submittingLabel="Submitting..."
    />,
  );
  return { ...result, onSuccess };
}

describe("ValidatedPopover", () => {
  it("close-while-loading resets loading state and ignores the late resolution", async () => {
    const d = deferred<string>();
    const fetchSpy = vi.fn(async () => d.promise);
    const { onSuccess } = renderHarness({ fetch: fetchSpy });

    // Type a key + submit
    fireEvent.change(screen.getByPlaceholderText("paste here"), { target: { value: "KEY-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));

    // Mid-flight: submitting label visible, button disabled.
    await waitFor(() => {
      const submitting = screen.getByRole("button", { name: "Submitting..." }) as HTMLButtonElement;
      expect(submitting.disabled).toBe(true);
    });

    // User dismisses the popover before fetch resolves.
    fireEvent.click(screen.getByTestId("popover-close"));

    // Loading must reset on close so the next open starts clean.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Submitting..." })).toBeNull();
    });

    // Late resolution must not call onSuccess (the submission token was
    // invalidated when we closed) and must not bring the popover back.
    await act(async () => {
      d.resolve("late");
      await Promise.resolve();
    });
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("close-while-loading suppresses a late rejection rather than re-populating error", async () => {
    const d = deferred<string>();
    const fetchSpy = vi.fn(async () => d.promise);
    renderHarness({ fetch: fetchSpy });

    fireEvent.change(screen.getByPlaceholderText("paste here"), { target: { value: "KEY-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    await waitFor(() => {
      const submitting = screen.getByRole("button", { name: "Submitting..." }) as HTMLButtonElement;
      expect(submitting.disabled).toBe(true);
    });

    fireEvent.click(screen.getByTestId("popover-close"));

    // Reject after close — the in-flight error must not leak into UI state.
    await act(async () => {
      d.reject(new Error("boom"));
      await Promise.resolve().catch(() => {});
    });

    // role=alert is the inline error <p> in ValidatedPopover; should be absent.
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
