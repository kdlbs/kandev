"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useCommands, useCommandPanelOpen } from "@/lib/commands/command-registry";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { listTasksByWorkspace } from "@/lib/api";
import { linkToSession } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { CommandPanelView } from "@/components/command-panel-footer";

function getFileName(filePath: string) {
  return filePath.split("/").pop() ?? filePath;
}

function useCommandPanelState() {
  const [mode, setMode] = useState<CommandPanelMode>("commands");
  const [search, setSearch] = useState("");
  const [inputCommand, setInputCommand] = useState<CommandItemType | null>(null);
  const [taskResults, setTaskResults] = useState<Task[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isSearchingFiles, setIsSearchingFiles] = useState(false);
  const [selectedValue, setSelectedValue] = useState("");
  return {
    mode,
    setMode,
    search,
    setSearch,
    inputCommand,
    setInputCommand,
    taskResults,
    setTaskResults,
    isSearching,
    setIsSearching,
    fileResults,
    setFileResults,
    isSearchingFiles,
    setIsSearchingFiles,
    selectedValue,
    setSelectedValue,
  };
}

function useCommandPanelEffectRefs() {
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const fileDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  return { debounceRef, abortRef, fileDebounceRef };
}

type FileSearchEffectOptions = {
  mode: CommandPanelMode;
  search: string;
  activeSessionId: string | null;
  setFileResults: (files: string[]) => void;
  setIsSearchingFiles: (searching: boolean) => void;
  setSelectedValue: (value: string) => void;
  fileDebounceRef: React.RefObject<ReturnType<typeof setTimeout> | null>;
};

function useFileSearchEffect(opts: FileSearchEffectOptions) {
  const {
    mode,
    search,
    activeSessionId,
    setFileResults,
    setIsSearchingFiles,
    setSelectedValue,
    fileDebounceRef,
  } = opts;
  useEffect(() => {
    if (mode !== "search-files" || !search.trim() || !activeSessionId) {
      setFileResults([]);
      setIsSearchingFiles(false);
      return;
    }
    setIsSearchingFiles(true);
    if (fileDebounceRef.current) clearTimeout(fileDebounceRef.current);
    let cancelled = false;
    fileDebounceRef.current = setTimeout(async () => {
      const client = getWebSocketClient();
      if (!client || cancelled) {
        if (!cancelled) setIsSearchingFiles(false);
        return;
      }
      try {
        const res = await searchWorkspaceFiles(client, activeSessionId, search.trim(), 10);
        if (!cancelled) {
          const files = res.files ?? [];
          setFileResults(files);
          if (files.length > 0) setSelectedValue(`__file:${files[0]}`);
        }
      } catch {
        if (!cancelled) setFileResults([]);
      } finally {
        if (!cancelled) setIsSearchingFiles(false);
      }
    }, 250);
    return () => {
      cancelled = true;
      if (fileDebounceRef.current) clearTimeout(fileDebounceRef.current);
    };
  }, [
    activeSessionId,
    fileDebounceRef,
    mode,
    search,
    setFileResults,
    setIsSearchingFiles,
    setSelectedValue,
  ]);
}

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);

function useInlineTaskSearchEffect(
  mode: CommandPanelMode,
  search: string,
  workspaceId: string | null,
  setTaskResults: (tasks: Task[]) => void,
  setIsSearching: (searching: boolean) => void,
) {
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (mode !== "commands") return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();

    if (!search.trim() || search.trim().length < 2) {
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    setIsSearching(true);
    debounceRef.current = setTimeout(async () => {
      if (!workspaceId) {
        setIsSearching(false);
        return;
      }
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await listTasksByWorkspace(
          workspaceId,
          { query: search.trim(), page: 1, pageSize: 5, includeArchived: true },
          { init: { signal: controller.signal } },
        );
        if (!controller.signal.aborted) {
          const tasks = res.tasks ?? [];
          tasks.sort((a, b) => {
            const aArchived = ARCHIVED_STATES.has(a.state) ? 1 : 0;
            const bArchived = ARCHIVED_STATES.has(b.state) ? 1 : 0;
            return aArchived - bArchived;
          });
          setTaskResults(tasks);
        }
      } catch {
        if (!controller.signal.aborted) setTaskResults([]);
      } finally {
        if (!controller.signal.aborted) setIsSearching(false);
      }
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, [mode, search, workspaceId, setTaskResults, setIsSearching]);
}

function useCommandPanelEffects(
  open: boolean,
  state: ReturnType<typeof useCommandPanelState>,
  workspaceId: string | null,
  activeSessionId: string | null,
) {
  const {
    mode,
    search,
    setMode,
    setSearch,
    setInputCommand,
    setTaskResults,
    setIsSearching,
    setFileResults,
    setIsSearchingFiles,
    setSelectedValue,
  } = state;
  const { fileDebounceRef } = useCommandPanelEffectRefs();
  useEffect(() => {
    if (!open) {
      const t = setTimeout(() => {
        setMode("commands");
        setSearch("");
        setInputCommand(null);
        setTaskResults([]);
        setFileResults([]);
        setSelectedValue("");
      }, 200);
      return () => clearTimeout(t);
    }
  }, [open, setMode, setSearch, setInputCommand, setTaskResults, setFileResults, setSelectedValue]);

  useInlineTaskSearchEffect(mode, search, workspaceId, setTaskResults, setIsSearching);

  useFileSearchEffect({
    mode,
    search,
    activeSessionId,
    setFileResults,
    setIsSearchingFiles,
    setSelectedValue,
    fileDebounceRef,
  });
}

function useCommandPanelHandlers(
  state: ReturnType<typeof useCommandPanelState>,
  setOpen: (open: boolean) => void,
  commands: CommandItemType[],
  kanbanSteps: { id: string; title: string; color: string }[],
  repositories: Array<{ id: string; local_path: string }>,
) {
  const { mode, search, inputCommand, setMode, setSearch, setInputCommand } = state;
  const router = useRouter();
  const { toast } = useToast();

  const grouped = useMemo(() => {
    const map = new Map<string, CommandItemType[]>();
    for (const cmd of commands) {
      const existing = map.get(cmd.group) ?? [];
      existing.push(cmd);
      map.set(cmd.group, existing);
    }
    return Array.from(map.entries()).sort(
      ([, a], [, b]) =>
        Math.min(...a.map((c) => c.priority ?? 100)) - Math.min(...b.map((c) => c.priority ?? 100)),
    );
  }, [commands]);

  const stepMap = useMemo(() => {
    const map = new Map<string, { name: string; color: string }>();
    for (const step of kanbanSteps) map.set(step.id, { name: step.title, color: step.color });
    return map;
  }, [kanbanSteps]);

  const repoMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const repo of repositories) map.set(repo.id, repo.local_path);
    return map;
  }, [repositories]);

  const handleSelect = useCallback(
    (cmd: CommandItemType) => {
      if (cmd.enterMode) {
        if (cmd.enterMode === "input") setInputCommand(cmd);
        setMode(cmd.enterMode);
        setSearch("");
        return;
      }
      if (cmd.action) {
        setOpen(false);
        cmd.action();
      }
    },
    [setOpen, setMode, setSearch, setInputCommand],
  );

  const handleTaskSelect = useCallback(
    (task: Task) => {
      setOpen(false);
      if (task.primary_session_id) router.push(linkToSession(task.primary_session_id));
      else toast({ title: "This task has no active session", variant: "default" });
    },
    [setOpen, router, toast],
  );

  const handleFileSelect = useCallback(
    (filePath: string) => {
      setOpen(false);
      useDockviewStore.getState().addFileEditorPanel(filePath, getFileName(filePath));
    },
    [setOpen],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (mode === "input" && e.key === "Enter" && search.trim() && inputCommand?.onInputSubmit) {
        e.preventDefault();
        setOpen(false);
        inputCommand.onInputSubmit(search.trim());
        return;
      }
      if (mode !== "commands" && e.key === "Backspace" && !search) {
        e.preventDefault();
        setMode("commands");
        setSearch("");
        setInputCommand(null);
      }
    },
    [mode, search, inputCommand, setOpen, setMode, setSearch, setInputCommand],
  );

  const goBack = useCallback(() => {
    setMode("commands");
    setSearch("");
    setInputCommand(null);
  }, [setMode, setSearch, setInputCommand]);

  return {
    grouped,
    stepMap,
    repoMap,
    handleSelect,
    handleTaskSelect,
    handleFileSelect,
    handleKeyDown,
    goBack,
  };
}

export function CommandPanel() {
  const { open, setOpen } = useCommandPanelOpen();
  const commands = useCommands();
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const repositories = useAppStore((s) =>
    workspaceId ? (s.repositories.itemsByWorkspaceId[workspaceId] ?? []) : [],
  );

  const state = useCommandPanelState();
  const {
    mode,
    search,
    inputCommand,
    taskResults,
    isSearching,
    fileResults,
    isSearchingFiles,
    selectedValue,
    setSelectedValue,
    setSearch,
  } = state;

  useCommandPanelEffects(open, state, workspaceId, activeSessionId);

  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);

  const toggleCommands = useCallback(() => setOpen(!openRef.current), [setOpen]);

  const openFileSearch = useCallback(() => {
    if (openRef.current && state.mode === "search-files") {
      setOpen(false);
    } else {
      state.setMode("search-files");
      state.setSearch("");
      setOpen(true);
    }
  }, [setOpen, state]);

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const searchShortcut = getShortcut("SEARCH", keyboardShortcuts);
  const fileSearchShortcut = getShortcut("FILE_SEARCH", keyboardShortcuts);

  useKeyboardShortcut(searchShortcut, toggleCommands);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL, toggleCommands);
  useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL_SHIFT, toggleCommands);
  useKeyboardShortcut(fileSearchShortcut, openFileSearch);

  const {
    grouped,
    stepMap,
    repoMap,
    handleSelect,
    handleTaskSelect,
    handleFileSelect,
    handleKeyDown,
    goBack,
  } = useCommandPanelHandlers(state, setOpen, commands, kanbanSteps, repositories);

  return (
    <CommandPanelView
      open={open}
      setOpen={setOpen}
      mode={mode}
      inputCommand={inputCommand}
      selectedValue={selectedValue}
      setSelectedValue={setSelectedValue}
      search={search}
      setSearch={setSearch}
      handleKeyDown={handleKeyDown}
      goBack={goBack}
      fileResults={fileResults}
      isSearchingFiles={isSearchingFiles}
      handleFileSelect={handleFileSelect}
      commands={commands}
      grouped={grouped}
      handleSelect={handleSelect}
      isSearching={isSearching}
      taskResults={taskResults}
      stepMap={stepMap}
      repoMap={repoMap}
      handleTaskSelect={handleTaskSelect}
    />
  );
}
