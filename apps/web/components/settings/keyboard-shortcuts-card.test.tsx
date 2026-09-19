import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ShortcutEntry } from "@/lib/keyboard/plugin-shortcuts";
import { KeyboardShortcutsCard } from "./keyboard-shortcuts-card";

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

afterEach(() => cleanup());

describe("KeyboardShortcutsCard", () => {
  it("renders core rows only while reporting conflicts with plugin bindings", () => {
    const onChange = vi.fn();
    const pluginEntry: ShortcutEntry = {
      source: "plugin",
      id: "plugin:session-cost:open-panel",
      label: "Session Cost: Open panel",
      default: { key: "k", modifiers: { ctrlOrCmd: true } },
      pluginId: "session-cost",
      keybindingId: "open-panel",
    };

    render(
      <KeyboardShortcutsCard overrides={{}} onChange={onChange} pluginEntries={[pluginEntry]} />,
    );

    expect(screen.getByTestId("shortcut-recorder-SEARCH")).toBeTruthy();
    expect(screen.queryByTestId(`shortcut-recorder-${pluginEntry.id}`)).toBeNull();
    expect(screen.getByTitle("Same shortcut as: Session Cost: Open panel")).toBeTruthy();
  });

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
