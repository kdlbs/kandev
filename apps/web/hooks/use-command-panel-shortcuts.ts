"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import type { CommandPanelMode } from "@/lib/commands/types";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";

type CommandPanelShortcutOptions = {
  open: boolean;
  setOpen: (open: boolean) => void;
  mode: CommandPanelMode;
  workspaceSearchAvailable: boolean;
  setMode: (mode: CommandPanelMode) => void;
  setSearch: (search: string) => void;
};

export function useCommandPanelShortcuts({
  open,
  setOpen,
  mode,
  workspaceSearchAvailable,
  setMode,
  setSearch,
}: CommandPanelShortcutOptions) {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);

  const openCommands = useCallback(() => {
    if (openRef.current && mode === "commands") {
      setOpen(false);
      return;
    }
    setMode("commands");
    setSearch("");
    setOpen(true);
  }, [mode, setMode, setOpen, setSearch]);

  const openFileSearch = useCallback(() => {
    if (openRef.current && mode === "search-files") {
      setOpen(false);
      return;
    }
    setMode("search-files");
    setSearch("");
    setOpen(true);
  }, [mode, setMode, setOpen, setSearch]);

  const keyboardShortcuts = useAppStore((state) => state.userSettings.keyboardShortcuts);
  useKeyboardShortcut(getShortcut("SEARCH", keyboardShortcuts), openCommands);
  useKeyboardShortcut(getShortcut("COMMAND_PANEL", keyboardShortcuts), openCommands);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL_SHIFT, openCommands);
  useKeyboardShortcut(getShortcut("FILE_SEARCH", keyboardShortcuts), openFileSearch, {
    enabled: workspaceSearchAvailable,
  });
}
