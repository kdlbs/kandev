import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ShortcutEntry } from "@/lib/keyboard/plugin-shortcuts";
import { UNBOUND_SHORTCUT } from "@/lib/keyboard/shortcut-overrides";
import { KeyboardShortcutsCard, ShortcutRecorder } from "./keyboard-shortcuts-card";

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

afterEach(() => cleanup());

const GITHUB_ACTION_LABEL = "Open GitHub";

describe("ShortcutRecorder keyboard accessibility", () => {
  function renderRecorder() {
    const onChange = vi.fn();
    const props = {
      shortcutId: "integration:github",
      label: GITHUB_ACTION_LABEL,
      defaultShortcut: UNBOUND_SHORTCUT,
      current: UNBOUND_SHORTCUT,
      onChange,
      onReset: vi.fn(),
    };
    return { ...render(<ShortcutRecorder {...props} />), props, onChange };
  }

  it("describes the unbound, recording and bound states while retaining the action name", () => {
    const view = renderRecorder();
    const button = screen.getByRole("button", {
      name: GITHUB_ACTION_LABEL,
      description: "Unbound",
    });
    expect(screen.getByRole("status").getAttribute("aria-live")).toBe("polite");
    fireEvent.click(button);
    expect(
      screen.getByRole("button", {
        name: GITHUB_ACTION_LABEL,
        description: "Press a key combo...",
      }),
    ).toBe(button);
    fireEvent.keyDown(window, { key: "g", ctrlKey: true, altKey: true });
    view.rerender(<ShortcutRecorder {...view.props} current={view.onChange.mock.lastCall![1]} />);
    expect(
      screen.getByRole("button", { name: GITHUB_ACTION_LABEL, description: "Ctrl+Alt+G" }),
    ).toBe(button);
  });

  it.each([false, true])(
    "ends integration recording without capturing focus traversal with Shift=%s",
    (shiftKey) => {
      const { onChange } = renderRecorder();
      const button = screen.getByRole("button", { name: GITHUB_ACTION_LABEL });
      fireEvent.click(button);
      const event = new KeyboardEvent("keydown", {
        key: "Tab",
        shiftKey,
        bubbles: true,
        cancelable: true,
      });
      fireEvent(window, event);
      expect(onChange).not.toHaveBeenCalled();
      expect(event.defaultPrevented).toBe(false);
      expect(button.getAttribute("data-shortcut-recording")).toBe("false");
    },
  );
});

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
