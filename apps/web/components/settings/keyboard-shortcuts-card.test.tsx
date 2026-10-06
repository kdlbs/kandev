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
  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.1, .3
  it("records and resets integration drafts while preserving unrelated overrides", () => {
    const onChange = vi.fn();
    const other = { SEARCH: { key: "o", modifiers: { ctrlOrCmd: true } } };
    const view = render(<KeyboardShortcutsCard overrides={other} onChange={onChange} />);
    fireEvent.click(screen.getByTestId("shortcut-recorder-integration:github"));
    fireEvent.keyDown(window, { key: "g", ctrlKey: true, altKey: true });
    expect(onChange).toHaveBeenLastCalledWith({
      ...other,
      "integration:github": { key: "g", modifiers: { ctrlOrCmd: true, alt: true } },
    });
    view.rerender(
      <KeyboardShortcutsCard overrides={onChange.mock.lastCall![0]} onChange={onChange} />,
    );
    fireEvent.click(
      screen
        .getByTestId("shortcut-recorder-integration:github")
        .parentElement!.querySelector("button[aria-label='Reset (clear shortcut)']")!,
    );
    expect(onChange).toHaveBeenLastCalledWith(other);
  });

  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.6
  it("reports integration conflicts with core shortcuts and plugin actions", () => {
    const pluginEntry: ShortcutEntry = {
      source: "plugin",
      id: "plugin:test:open",
      label: "Test: Open",
      default: { key: "k", modifiers: { ctrlOrCmd: true } },
      pluginId: "test",
      keybindingId: "open",
    };
    render(
      <KeyboardShortcutsCard
        overrides={{ "integration:github": { key: "k", modifiers: { ctrlOrCmd: true } } }}
        onChange={vi.fn()}
        pluginEntries={[pluginEntry]}
      />,
    );
    expect(screen.getByTitle("Same shortcut as: Open GitHub, Test: Open")).toBeTruthy();
    expect(screen.queryByTestId("shortcut-recorder-plugin:test:open")).toBeNull();
  });
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
