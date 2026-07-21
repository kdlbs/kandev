import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KeyboardShortcutsCard } from "./keyboard-shortcuts-card";

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

afterEach(() => cleanup());

describe("KeyboardShortcutsCard", () => {
  it("updates its route draft without owning persistence", () => {
    const onChange = vi.fn();
    render(<KeyboardShortcutsCard overrides={{}} onChange={onChange} />);

    fireEvent.click(screen.getByTestId("shortcut-recorder-SEARCH"));
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });

    expect(onChange).toHaveBeenCalledWith({
      SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
    });
  });
});
