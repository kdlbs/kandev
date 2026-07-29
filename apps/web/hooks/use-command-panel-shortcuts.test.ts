import { act, cleanup, renderHook } from "@testing-library/react";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type ShortcutRegistration = {
  shortcut: KeyboardShortcut;
  callback: () => void;
  enabled: boolean;
};

const registrations: ShortcutRegistration[] = [];

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: (
    shortcut: KeyboardShortcut,
    callback: () => void,
    options?: { enabled?: boolean },
  ) => {
    registrations.push({ shortcut, callback, enabled: options?.enabled ?? true });
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ userSettings: { keyboardShortcuts: {} } }),
}));

import { useCommandPanelShortcuts } from "./use-command-panel-shortcuts";

function findShortcut(key: string, shift = false) {
  return registrations.find(
    ({ shortcut }) => shortcut.key === key && Boolean(shortcut.modifiers?.shift) === shift,
  );
}

beforeEach(() => registrations.splice(0));
afterEach(() => cleanup());

describe("useCommandPanelShortcuts", () => {
  it("switches an open file palette to commands instead of closing it", () => {
    const setMode = vi.fn();
    const setOpen = vi.fn();
    const setSearch = vi.fn();
    renderHook(() =>
      useCommandPanelShortcuts({
        open: true,
        mode: "search-files",
        workspaceSearchAvailable: true,
        setMode,
        setOpen,
        setSearch,
      }),
    );

    act(() => findShortcut("k")?.callback());

    expect(setMode).toHaveBeenCalledWith("commands");
    expect(setSearch).toHaveBeenCalledWith("");
    expect(setOpen).toHaveBeenCalledWith(true);
  });

  it("does not register file search outside an active task workbench", () => {
    const options = {
      open: false,
      mode: "commands" as const,
      workspaceSearchAvailable: false,
      setMode: vi.fn(),
      setOpen: vi.fn(),
      setSearch: vi.fn(),
    };

    renderHook(() => useCommandPanelShortcuts(options));

    expect(findShortcut("k", true)?.enabled).toBe(false);
    expect(findShortcut("k")?.enabled).toBe(true);
  });
});
