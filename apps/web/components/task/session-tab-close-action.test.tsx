import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SessionTabCloseAction } from "./session-tab-close-action";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(() => cleanup());

describe("SessionTabCloseAction", () => {
  it("renders an operable X and dispatches close when idle", () => {
    const onClose = vi.fn();
    render(<SessionTabCloseAction sessionId="s1" isDeleting={false} onClose={onClose} />);

    const button = screen.getByRole("button", { name: "common:deleteSession" });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(button.getAttribute("aria-busy")).toBe("false");
    expect(screen.queryByRole("status")).toBeNull();

    fireEvent.click(button);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders a disabled spinner and blocks close while deleting", () => {
    const onClose = vi.fn();
    render(<SessionTabCloseAction sessionId="s1" isDeleting onClose={onClose} />);

    const button = screen.getByRole("button", { name: "common:deleteSession" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("status")).toBeTruthy();

    fireEvent.click(button);

    expect(onClose).not.toHaveBeenCalled();
  });
});
