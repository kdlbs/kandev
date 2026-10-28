import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ShortcutEntry } from "@/lib/keyboard/plugin-shortcuts";
import { UNBOUND_SHORTCUT } from "@/lib/keyboard/shortcut-overrides";
import {
  buildConflictLabels,
  KeyboardShortcutsCard,
  ShortcutRecorder,
} from "./keyboard-shortcuts-card";
import type { ShortcutConflictGroup } from "@/lib/keyboard/shortcut-conflicts";

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5
describe("direct conflict labels", () => {
  const entries: ShortcutEntry[] = ["Mod", "Ctrl", "Meta"].map((label) => ({
    source: "plugin",
    id: `plugin:test:${label}`,
    label,
    default: { key: "q" },
    pluginId: "test",
    keybindingId: label,
  }));

  it("accumulates direct peers once without labeling disjoint actions", () => {
    const groups = [
      { entries: [entries[0], entries[1]] },
      { entries: [entries[0], entries[2]] },
      { entries: [entries[0], entries[1]] },
    ] satisfies ShortcutConflictGroup[];
    expect([...buildConflictLabels(groups)]).toEqual([
      [entries[0].id, ["Ctrl", "Meta"]],
      [entries[1].id, ["Mod"]],
      [entries[2].id, ["Mod"]],
    ]);
  });

  it("renders both true plugin peers on a core Mod row on Mac", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel");
    render(
      <KeyboardShortcutsCard
        overrides={{ SEARCH: { key: "q", modifiers: { ctrlOrCmd: true } } }}
        onChange={vi.fn()}
        pluginEntries={entries.slice(1).map((entry, index) => ({
          ...entry,
          default: { key: "q", modifiers: index === 0 ? { ctrl: true } : { cmd: true } },
        }))}
      />,
    );
    expect(screen.getByTitle("Same shortcut as: Ctrl, Meta")).toBeTruthy();
  });
});

const GITHUB_ACTION_LABEL = "Open GitHub";
const GITHUB_RECORDER = "shortcut-recorder-integration:github";
const SEARCH_RECORDER = "shortcut-recorder-SEARCH";
const SEARCH_LABEL = "Search shortcuts";

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
    fireEvent.click(screen.getByTestId(GITHUB_RECORDER));
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
        .getByTestId(GITHUB_RECORDER)
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

    expect(screen.getByTestId(SEARCH_RECORDER)).toBeTruthy();
    expect(screen.queryByTestId(`shortcut-recorder-${pluginEntry.id}`)).toBeNull();
    expect(screen.getByTitle("Same shortcut as: Session Cost: Open panel")).toBeTruthy();
  });

  it("updates its route draft without owning persistence", () => {
    const onChange = vi.fn();
    render(<KeyboardShortcutsCard overrides={{}} onChange={onChange} />);

    fireEvent.click(screen.getByTestId(SEARCH_RECORDER));
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });

    expect(onChange).toHaveBeenCalledWith({
      SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
    });
  });
});

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.1
it("filters the mounted settings card by a translated action label", () => {
  render(<KeyboardShortcutsCard overrides={{}} onChange={vi.fn()} />);
  const search = screen.getByRole("textbox", { name: SEARCH_LABEL });
  fireEvent.change(search, { target: { value: "  gItHuB  " } });
  expect(screen.getByTestId(GITHUB_RECORDER)).toBeTruthy();
  expect(screen.queryByTestId(SEARCH_RECORDER)).toBeNull();
});

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.1, .2, .3, .4
it("restores rows and pending overrides while retaining conflicts with hidden peers", () => {
  const onChange = vi.fn();
  const pluginEntry: ShortcutEntry = {
    source: "plugin",
    id: "plugin:test:open",
    label: "Test plugin action",
    default: { key: "k", modifiers: { ctrlOrCmd: true } },
    pluginId: "test",
    keybindingId: "open",
  };
  const overrides = { "integration:github": { key: "k", modifiers: { ctrlOrCmd: true } } };
  render(
    <KeyboardShortcutsCard
      overrides={overrides}
      onChange={onChange}
      pluginEntries={[pluginEntry]}
    />,
  );
  const search = screen.getByRole("textbox", { name: SEARCH_LABEL });
  const initialRows = screen.getAllByTestId(/^shortcut-recorder-/).map((row) => row.dataset.testid);
  fireEvent.change(search, { target: { value: "  CoMmAnD PaNeL  " } });
  expect(screen.getByTestId(SEARCH_RECORDER)).toBeTruthy();
  expect(screen.queryByTestId(GITHUB_RECORDER)).toBeNull();
  expect(screen.getByTitle("Same shortcut as: Open GitHub, Test plugin action")).toBeTruthy();
  fireEvent.change(search, { target: { value: "  GITHUB  " } });
  const github = screen.getByTestId(GITHUB_RECORDER);
  expect(github.getAttribute("data-settings-dirty")).toBe("true");
  expect(screen.getByTitle("Same shortcut as: Command Panel, Test plugin action")).toBeTruthy();
  fireEvent.change(search, { target: { value: "no such shortcut" } });
  expect(screen.getAllByText("No matching shortcuts")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expect(screen.getAllByTestId(/^shortcut-recorder-/).map((row) => row.dataset.testid)).toEqual(
    initialRows,
  );
  expect(screen.getByTestId(GITHUB_RECORDER).textContent).toContain("K");
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.change(search, { target: { value: "   " } });
  expect(screen.getAllByTestId(/^shortcut-recorder-/)).toHaveLength(initialRows.length);
});

it("filters dynamically supplied integration labels", () => {
  render(
    <KeyboardShortcutsCard
      overrides={{}}
      onChange={vi.fn()}
      integrationEntries={[
        {
          source: "integration",
          id: "integration:dynamic",
          label: "Open Example Destination",
          default: UNBOUND_SHORTCUT,
          href: "/example",
        },
      ]}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: SEARCH_LABEL }), {
    target: { value: "  eXAMple  " },
  });
  expect(screen.getByTestId("shortcut-recorder-integration:dynamic")).toBeTruthy();
  expect(screen.queryByTestId(SEARCH_RECORDER)).toBeNull();
});

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.3
it("stops recording and lets filter typing through without changing shortcuts", () => {
  const onChange = vi.fn();
  render(<KeyboardShortcutsCard overrides={{}} onChange={onChange} />);
  const recorder = screen.getByTestId(SEARCH_RECORDER);
  recorder.focus();
  fireEvent.click(recorder);
  expect(recorder.getAttribute("data-shortcut-recording")).toBe("true");
  const search = screen.getByRole("textbox", { name: SEARCH_LABEL });
  search.focus();
  const event = new KeyboardEvent("keydown", { key: "g", bubbles: true, cancelable: true });
  fireEvent(search, event);
  expect(event.defaultPrevented).toBe(false);
  expect(recorder.getAttribute("data-shortcut-recording")).toBe("false");
  expect(onChange).not.toHaveBeenCalled();
});

// @covers AC-UI-SHORTCUT-SETTINGS-FILTER-001.2, .5
it("returns keyboard focus to search after clearing", () => {
  render(<KeyboardShortcutsCard overrides={{}} onChange={vi.fn()} />);
  const search = screen.getByRole("textbox", { name: SEARCH_LABEL });
  fireEvent.change(search, { target: { value: "github" } });
  const clear = screen.getByRole("button", { name: "Clear search" });
  clear.focus();
  fireEvent.click(clear);
  expect(document.activeElement).toBe(search);
});
