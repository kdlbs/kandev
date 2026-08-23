"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { useCommands, useCommandPanelOpen } from "@/lib/commands/command-registry";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import {
  findFirstMatchingCommand,
  selectCommandSearchResult,
  selectContentSearchResult,
} from "@/lib/commands/search";
import { useCommandPanelShortcuts } from "@/hooks/use-command-panel-shortcuts";
import { useContentSearchResultOpener } from "@/hooks/use-content-search-result-opener";
import { useWorkspaceContentSearch } from "@/hooks/domains/session/use-workspace-content-search";
import { useAppStore } from "@/components/state-provider";
import {
  isCommandPanelScopeMode,
  type CommandPanelScopeMode,
} from "@/components/command-panel-scope-switcher";

import { listTasksByWorkspace } from "@/lib/api";
import type { Task } from "@/lib/types/http";
import type { FileSearchResult } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getContentSearchResultValue } from "@/components/workspace-content-search";
import { getFileName } from "@/lib/utils/file-path";
import { isTaskWorkspaceSearchAvailable } from "@/lib/commands/task-workspace-search";
import { useCommandPanelTaskNavigation } from "@/hooks/use-command-panel-task-navigation";
import {
  CommandPanelView,
  MODE_COMMANDS,
  MODE_SEARCH_CONTENT,
  MODE_SEARCH_FILES,
  getFileResultValue,
  getTaskResultValue,
} from "@/components/command-panel-footer";

function useCommandPanelState(mode: CommandPanelMode, setMode: (mode: CommandPanelMode) => void) {
  const [search, setSearch] = useState("");
  const [inputCommand, setInputCommand] = useState<CommandItemType | null>(null);
  const [taskResults, setTaskResults] = useState<Task[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [fileResults, setFileResults] = useState<FileSearchResult[]>([]);
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

type FileSearchEffectOptions = {
  mode: CommandPanelMode;
  search: string;
  workspaceSearchAvailable: boolean;
  activeSessionId: string | null;
  setFileResults: (files: FileSearchResult[]) => void;
  setIsSearchingFiles: (searching: boolean) => void;
  fileDebounceRef: React.RefObject<ReturnType<typeof setTimeout> | null>;
};

function useFileSearchEffect(opts: FileSearchEffectOptions) {
  const {
    mode,
    search,
    workspaceSearchAvailable,
    activeSessionId,
    setFileResults,
    setIsSearchingFiles,
    fileDebounceRef,
  } = opts;
  useEffect(() => {
    if (
      !workspaceSearchAvailable ||
      mode !== MODE_SEARCH_FILES ||
      !search.trim() ||
      !activeSessionId
    ) {
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
          const results = res.results ?? (res.files ?? []).map((path) => ({ path }));
          setFileResults(results);
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
    workspaceSearchAvailable,
  ]);
}

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);

function resolveVisibleStepIds(steps: { id: string; show_in_command_panel?: boolean }[]) {
  if (steps.length === 0) return null; // no steps loaded yet — don't filter
  return new Set(steps.filter((s) => s.show_in_command_panel !== false).map((s) => s.id));
}

type InlineTaskSearchOptions = {
  mode: CommandPanelMode;
  search: string;
  open: boolean;
  workspaceId: string | null;
  steps: { id: string; position: number; show_in_command_panel?: boolean }[];
  setTaskResults: (tasks: Task[]) => void;
  setIsSearching: (searching: boolean) => void;
};

function useStepMaps(steps: InlineTaskSearchOptions["steps"]) {
  const visibleStepIds = useMemo(() => resolveVisibleStepIds(steps), [steps]);
  const stepPositionMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const step of steps) map.set(step.id, step.position);
    return map;
  }, [steps]);
  return { visibleStepIds, stepPositionMap };
}

function useInlineTaskSearchEffect(opts: InlineTaskSearchOptions) {
  const { mode, search, open, workspaceId, steps, setTaskResults, setIsSearching } = opts;
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const { visibleStepIds, stepPositionMap } = useStepMaps(steps);

  useEffect(() => {
    if (mode !== MODE_COMMANDS) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();

    // No search: load active tasks (excluding backlog + done steps)
    if (!search.trim()) {
      if (!open || !workspaceId) {
        setTaskResults([]);
        setIsSearching(false);
        return;
      }
      setIsSearching(true);
      const controller = new AbortController();
      abortRef.current = controller;
      listTasksByWorkspace(
        workspaceId,
        { page: 1, pageSize: 20 },
        { init: { signal: controller.signal } },
      )
        .then((res) => {
          if (controller.signal.aborted) return;
          const tasks = (res.tasks ?? []).filter(
            (t) =>
              (!visibleStepIds || visibleStepIds.has(t.workflow_step_id)) &&
              !ARCHIVED_STATES.has(t.state),
          );
          tasks.sort(
            (a, b) =>
              (stepPositionMap.get(a.workflow_step_id) ?? 99) -
              (stepPositionMap.get(b.workflow_step_id) ?? 99),
          );
          setTaskResults(tasks);
        })
        .catch(() => {
          if (!controller.signal.aborted) setTaskResults([]);
        })
        .finally(() => {
          if (!controller.signal.aborted) setIsSearching(false);
        });
      return () => {
        controller.abort();
      };
    }

    // Search with < 2 chars: clear results
    if (search.trim().length < 2) {
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    // Search: query API including archived
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
  }, [
    mode,
    search,
    open,
    workspaceId,
    visibleStepIds,
    stepPositionMap,
    setTaskResults,
    setIsSearching,
  ]);
}

type CommandPanelEffectsOptions = {
  open: boolean;
  state: ReturnType<typeof useCommandPanelState>;
  workspaceId: string | null;
  activeSessionId: string | null;
  workspaceSearchAvailable: boolean;
  steps: { id: string; position: number; show_in_command_panel?: boolean }[];
  modeRequestVersion: number;
};

function useCommandPanelEffects(options: CommandPanelEffectsOptions) {
  const {
    open,
    state,
    workspaceId,
    activeSessionId,
    workspaceSearchAvailable,
    steps,
    modeRequestVersion,
  } = options;
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
  const fileDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const previousRequestVersion = useRef(modeRequestVersion);
  useEffect(() => {
    if (previousRequestVersion.current === modeRequestVersion) return;
    previousRequestVersion.current = modeRequestVersion;
    setSearch("");
    setInputCommand(null);
    setTaskResults([]);
    setFileResults([]);
    setSelectedValue("");
  }, [
    modeRequestVersion,
    setFileResults,
    setInputCommand,
    setSearch,
    setSelectedValue,
    setTaskResults,
  ]);
  useEffect(() => {
    if (!open) {
      const t = setTimeout(() => {
        setMode(MODE_COMMANDS);
        setSearch("");
        setInputCommand(null);
        setTaskResults([]);
        setFileResults([]);
        setSelectedValue("");
      }, 200);
      return () => clearTimeout(t);
    }
  }, [open, setMode, setSearch, setInputCommand, setTaskResults, setFileResults, setSelectedValue]);

  useInlineTaskSearchEffect({
    mode,
    search,
    open,
    workspaceId,
    steps,
    setTaskResults,
    setIsSearching,
  });

  useFileSearchEffect({
    mode,
    search,
    workspaceSearchAvailable,
    activeSessionId,
    setFileResults,
    setIsSearchingFiles,
    fileDebounceRef,
  });
}

function useFirstResultSelection(
  open: boolean,
  state: ReturnType<typeof useCommandPanelState>,
  commands: CommandItemType[],
  contentResults: ReturnType<typeof useWorkspaceContentSearch>["results"],
) {
  const { mode, search, taskResults, fileResults, setSelectedValue } = state;
  const previousCommandsRef = useRef(commands);

  // `search` intentionally re-applies the first result while debounced results are loading.
  useEffect(() => {
    const commandsChanged = previousCommandsRef.current !== commands;
    previousCommandsRef.current = commands;
    if (!open) return;

    if (mode === MODE_COMMANDS) {
      const firstTask = taskResults[0];
      if (commandsChanged) {
        const taskValues = taskResults.map(getTaskResultValue);
        setSelectedValue((current) =>
          selectCommandSearchResult(commands, search, taskValues, current),
        );
        return;
      }
      if (firstTask) {
        setSelectedValue(getTaskResultValue(firstTask));
        return;
      }
      if (search.trim()) {
        setSelectedValue(findFirstMatchingCommand(commands, search)?.id ?? "");
        return;
      }
      setSelectedValue((current) => (current.startsWith("__task:") ? "" : current));
      return;
    }

    if (mode === MODE_SEARCH_FILES) {
      const firstFile = fileResults[0];
      setSelectedValue(firstFile ? getFileResultValue(firstFile.path) : "");
      return;
    }

    if (mode === MODE_SEARCH_CONTENT) {
      const values = contentResults.map(getContentSearchResultValue);
      setSelectedValue((current) => selectContentSearchResult(values, current));
      return;
    }

    setSelectedValue("");
  }, [commands, contentResults, fileResults, mode, open, search, setSelectedValue, taskResults]);
}

type CommandPanelHandlerOptions = {
  state: ReturnType<typeof useCommandPanelState>;
  setOpen: (open: boolean) => void;
  commands: CommandItemType[];
  kanbanSteps: { id: string; title: string; color: string }[];
  repositories: Array<{ id: string; local_path: string }>;
  onTaskSelect: (task: Task) => void;
};

function useCommandPanelHandlers({
  state,
  setOpen,
  commands,
  kanbanSteps,
  repositories,
  onTaskSelect,
}: CommandPanelHandlerOptions) {
  const { mode, search, inputCommand, setMode, setSearch, setInputCommand } = state;

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
        if (!cmd.keepOpen) setOpen(false);
        cmd.action();
      }
    },
    [setOpen, setMode, setSearch, setInputCommand],
  );

  const handleTaskSelect = useCallback(
    (task: Task) => {
      setOpen(false);
      onTaskSelect(task);
    },
    [onTaskSelect, setOpen],
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
      if (!isCommandPanelScopeMode(mode) && e.key === "Backspace" && !search) {
        e.preventDefault();
        setMode(MODE_COMMANDS);
        setSearch("");
        setInputCommand(null);
      }
    },
    [mode, search, inputCommand, setOpen, setMode, setSearch, setInputCommand],
  );

  const onScopeChange = (nextMode: CommandPanelScopeMode) => {
    setMode(nextMode);
    setInputCommand(null);
  };

  const goBack = useCallback(() => {
    setMode(MODE_COMMANDS);
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
    onScopeChange,
    goBack,
  };
}

export function CommandPanel() {
  const { open, setOpen, mode: panelMode, setMode, modeRequestVersion } = useCommandPanelOpen();
  const commands = useCommands();
  const pathname = usePathname();
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const workspaceSearchAvailable = useAppStore((state) =>
    isTaskWorkspaceSearchAvailable(state, pathname),
  );
  const worktreePath = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId]?.worktree_path ?? null) : null,
  );
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repositories = workspaceId ? (reposByWorkspace[workspaceId] ?? []) : [];
  const handleTaskNavigation = useCommandPanelTaskNavigation(pathname, activeTaskId);

  const state = useCommandPanelState(panelMode, setMode);
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

  useCommandPanelEffects({
    open,
    state,
    workspaceId,
    activeSessionId,
    workspaceSearchAvailable,
    steps: kanbanSteps,
    modeRequestVersion,
  });
  const {
    results: contentResults,
    isSearching: isSearchingContent,
    error: contentSearchError,
  } = useWorkspaceContentSearch({
    enabled: open && workspaceSearchAvailable && mode === MODE_SEARCH_CONTENT,
    query: search,
    sessionId: activeSessionId,
  });
  useFirstResultSelection(open, state, commands, contentResults);

  useCommandPanelShortcuts({
    open,
    setOpen,
    mode,
    workspaceSearchAvailable,
    setMode,
    setSearch,
  });

  const handlers = useCommandPanelHandlers({
    state,
    setOpen,
    commands,
    kanbanSteps,
    repositories,
    onTaskSelect: handleTaskNavigation,
  });
  const handleContentSelect = useContentSearchResultOpener(setOpen, worktreePath, activeSessionId);

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
      handleKeyDown={handlers.handleKeyDown}
      onScopeChange={handlers.onScopeChange}
      goBack={handlers.goBack}
      fileResults={fileResults}
      isSearchingFiles={isSearchingFiles}
      handleFileSelect={handlers.handleFileSelect}
      contentResults={contentResults}
      isSearchingContent={isSearchingContent}
      contentSearchError={contentSearchError}
      activeSessionId={activeSessionId}
      workspaceSearchAvailable={workspaceSearchAvailable}
      handleContentSelect={handleContentSelect}
      commands={commands}
      grouped={handlers.grouped}
      handleSelect={handlers.handleSelect}
      isSearching={isSearching}
      taskResults={taskResults}
      stepMap={handlers.stepMap}
      repoMap={handlers.repoMap}
      handleTaskSelect={handlers.handleTaskSelect}
    />
  );
}
