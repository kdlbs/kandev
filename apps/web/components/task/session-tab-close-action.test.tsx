import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SessionTabCloseAction } from "./session-tab-close-action";

afterEach(() => cleanup());

describe("SessionTabCloseAction", () => {
  it("renders an operable close action without delete progress state", () => {
    const onClose = vi.fn();
    render(<SessionTabCloseAction sessionId="s1" onClose={onClose} />);

    const button = screen.getByRole("button", { name: "Close" });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(button.hasAttribute("aria-busy")).toBe(false);
    expect(screen.queryByRole("status")).toBeNull();

    fireEvent.click(button);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("keeps pointer interaction on the close action out of the tab trigger", () => {
    const onClose = vi.fn();
    const onPointerDown = vi.fn();
    render(
      <div onPointerDown={onPointerDown}>
        <SessionTabCloseAction sessionId="s1" onClose={onClose} />
      </div>,
    );

    const button = screen.getByRole("button", { name: "Close" });
    fireEvent.pointerDown(button);
    expect(onPointerDown).not.toHaveBeenCalled();

    fireEvent.click(button);

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
