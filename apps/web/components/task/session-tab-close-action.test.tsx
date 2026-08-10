import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SessionTabCloseAction } from "./session-tab-close-action";

afterEach(() => cleanup());

describe("SessionTabCloseAction", () => {
  it("renders an operable X and dispatches close when idle", () => {
    const onClose = vi.fn();
    render(<SessionTabCloseAction sessionId="s1" isDeleting={false} onClose={onClose} />);

    const button = screen.getByRole("button", { name: "Delete session" });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(button.getAttribute("aria-busy")).toBe("false");
    expect(screen.queryByRole("status")).toBeNull();

    fireEvent.click(button);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders a disabled spinner and blocks close while deleting", () => {
    const onClose = vi.fn();
    render(<SessionTabCloseAction sessionId="s1" isDeleting onClose={onClose} />);

    const button = screen.getByRole("button", { name: "Delete session" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
    const spinner = screen.getByRole("status");
    expect(spinner.classList.contains("animate-spin")).toBe(true);
    expect(spinner.classList.contains("rounded-full")).toBe(true);
    expect(spinner.classList.contains("border-2")).toBe(true);
    expect(spinner.classList.contains("border-t-muted-foreground")).toBe(true);
    expect(spinner.classList.contains("spinner-grid")).toBe(false);
    expect(spinner.querySelectorAll(".spinner-grid-cube")).toHaveLength(0);

    fireEvent.click(button);

    expect(onClose).not.toHaveBeenCalled();
  });
});
