import { useRef } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useInlineEditFocus } from "./use-inline-edit-focus";

function Editor({
  onLeave,
  identity = "first",
  autoFocus = true,
}: {
  onLeave: () => void;
  identity?: string;
  autoFocus?: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const { onBlur, cancelPendingBlur } = useInlineEditFocus({
    active: true,
    identity,
    inputRef,
    containerRef,
    onLeave,
  });
  return (
    <>
      <div ref={containerRef}>
        <input
          aria-label="Draft"
          ref={inputRef}
          autoFocus={autoFocus}
          onBlur={onBlur}
          onKeyDown={cancelPendingBlur}
        />
        <button>Inside</button>
      </div>
      <button>Outside</button>
      <div data-testid="blank">Blank area</div>
    </>
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function flush() {
  act(() => vi.runAllTimers());
}

// @covers AC-UI-EDIT-FOCUS-001.1, AC-UI-EDIT-FOCUS-001.3, AC-UI-EDIT-FOCUS-001.4
it.each(["pointer", "focus"])("ignores outside %s events until initial focus", (kind) => {
  const leave = vi.fn();
  render(<Editor onLeave={leave} autoFocus={false} />);
  const outside = screen.getByRole("button", { name: "Outside" });
  const moveOutside = () => {
    if (kind === "pointer") fireEvent.pointerDown(outside);
    else fireEvent.focusIn(outside);
    flush();
  };
  moveOutside();
  expect(leave).not.toHaveBeenCalled();
  act(() => screen.getByRole("textbox").focus());
  moveOutside();
  expect(leave).toHaveBeenCalledTimes(1);
});

describe("inline edit focus ownership", () => {
  it.each(["visible", "hidden"] as const)("preserves an external blur while %s", (visibility) => {
    vi.spyOn(document, "visibilityState", "get").mockReturnValue(visibility);
    vi.mocked(document.hasFocus).mockReturnValue(false);
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    fireEvent.blur(screen.getByRole("textbox"));
    flush();
    expect(leave).not.toHaveBeenCalled();
    vi.mocked(document.hasFocus).mockReturnValue(true);
    fireEvent.focus(screen.getByRole("textbox"));
    expect(leave).not.toHaveBeenCalled();
  });

  it("settles focus after a null-target blur before deciding", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    fireEvent.blur(screen.getByRole("textbox"));
    vi.mocked(document.hasFocus).mockReturnValue(false);
    flush();
    expect(leave).not.toHaveBeenCalled();
  });

  it("completes once for an internal focus destination and its focus event", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    const outside = screen.getByRole("button", { name: "Outside" });
    fireEvent.blur(screen.getByRole("textbox"), { relatedTarget: outside });
    fireEvent.focusIn(outside);
    flush();
    expect(leave).toHaveBeenCalledTimes(1);
  });

  it("preserves moves inside the edit container", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    const inside = screen.getByRole("button", { name: "Inside" });
    fireEvent.blur(screen.getByRole("textbox"), { relatedTarget: inside });
    fireEvent.focusIn(inside);
    flush();
    expect(leave).not.toHaveBeenCalled();
  });

  it("completes on a nonfocusable outside click even without a related target", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    fireEvent.pointerDown(screen.getByTestId("blank"));
    fireEvent.blur(screen.getByRole("textbox"));
    flush();
    expect(leave).toHaveBeenCalledTimes(1);
  });

  it("completes on an outside click after returning without refocusing the input", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    vi.mocked(document.hasFocus).mockReturnValue(false);
    fireEvent.blur(screen.getByRole("textbox"));
    flush();
    expect(leave).not.toHaveBeenCalled();
    vi.mocked(document.hasFocus).mockReturnValue(true);
    fireEvent.pointerDown(screen.getByTestId("blank"));
    flush();
    expect(leave).toHaveBeenCalledTimes(1);
  });

  it("treats an iframe in the same document as an internal destination", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    const frame = document.createElement("iframe");
    fireEvent.blur(screen.getByRole("textbox"), { relatedTarget: frame });
    expect(leave).toHaveBeenCalledTimes(1);
  });

  it("does not finish a newer edit from an old deferred blur", () => {
    const oldLeave = vi.fn();
    const newLeave = vi.fn();
    const view = render(<Editor onLeave={oldLeave} />);
    fireEvent.pointerDown(screen.getByTestId("blank"));
    view.rerender(<Editor identity="second" onLeave={newLeave} />);
    flush();
    expect(oldLeave).not.toHaveBeenCalled();
    expect(newLeave).not.toHaveBeenCalled();
    fireEvent.focusIn(screen.getByRole("button", { name: "Outside" }));
    expect(newLeave).toHaveBeenCalledTimes(1);
  });

  it("invalidates pending work and listeners on unmount", () => {
    const leave = vi.fn();
    const view = render(<Editor onLeave={leave} />);
    fireEvent.pointerDown(screen.getByTestId("blank"));
    view.unmount();
    flush();
    fireEvent.focusIn(document.body);
    expect(leave).not.toHaveBeenCalled();
  });

  it("does not complete again after an explicit keyboard exit", () => {
    const leave = vi.fn();
    render(<Editor onLeave={leave} />);
    const input = screen.getByRole("textbox");
    fireEvent.pointerDown(screen.getByTestId("blank"));
    fireEvent.keyDown(input, { key: "Escape" });
    fireEvent.blur(input, { relatedTarget: document.body });
    flush();
    expect(leave).not.toHaveBeenCalled();
  });
});
