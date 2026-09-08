"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { useCommands, useCommandPanelOpen } from "@/lib/commands/command-registry";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import { selectCommandSearchResult, selectContentSearchResult } from "@/lib/commands/search";
import { useCommandPanelShortcuts } from "@/hooks/use-command-panel-shortcuts";
import { useContentSearchResultOpener } from "@/hooks/use-content-search-result-opener";
import { useWorkspaceContentSearch } from "@/hooks/domains/session/use-workspace-content-search";
import { useAppStore } from "@/components/state-provider";
import {
  isCommandPanelScopeMode,
  type CommandPanelScopeMode,
} from "@/components/command-panel-scope-switcher";

import type { Task } from "@/lib/types/http";
import type { FileSearchResult } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getContentSearchResultValue } from "@/components/workspace-content-search";
import { getFileName } from "@/lib/utils/file-path";
import { isTaskWorkspaceSearchAvailable } from "@/lib/commands/task-workspace-search";
import { useCommandPanelTaskNavigation } from "@/hooks/use-command-panel-task-navigation";
import { useInlineTaskSearchEffect } from "@/hooks/use-command-panel-task-results";
import {
  CommandPanelView,
  MODE_COMMANDS,
  MODE_SEARCH_CONTENT,
  MODE_SEARCH_FILES,
  MODE_SEARCH_TASKS,
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

  // Every branch is a functional update, so a re-registration of the command
  // list or a late batch of results never discards a selection the user moved.
  useEffect(() => {
    if (!open) return;

    if (mode === MODE_COMMANDS) {
      // Matching commands render above the task preview once there is a query,
      // so the default highlight has to follow that order: Enter on "archive"
      // must run the Archive command, not the first task the query fuzzy-matched.
      // Task results arrive 300ms behind the keystroke, so this stays a
      // functional update: a row the user arrow-keyed to in the meantime must
      // survive the results landing rather than snap back to the default.
      const commandsLeadResults = Boolean(search.trim());
      const taskResultValues = taskResults.map(getTaskResultValue);
      setSelectedValue((current) =>
        selectCommandSearchResult({
          commands,
          search,
          taskResultValues,
          preferredValue: current,
          commandsLeadResults,
        }),
      );
      return;
    }

    if (mode === MODE_SEARCH_TASKS) {
      const taskResultValues = taskResults.map(getTaskResultValue);
      setSelectedValue((current) =>
        current && taskResultValues.includes(current) ? current : (taskResultValues[0] ?? ""),
      );
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

function findLastWorkflowStepId(steps: { id: string; position: number }[]) {
  let lastStep: { id: string; position: number } | undefined;
  for (const step of steps) {
    if (!lastStep || step.position > lastStep.position) lastStep = step;
  }
  return lastStep?.id;
}

function useCommandPanelLiveTasks() {
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const kanbanSnapshots = useAppStore((state) => state.kanbanMulti?.snapshots ?? {});
  return useMemo(() => {
    const tasks = new Map<string, (typeof kanbanTasks)[number]>();
    const lastStepIdByWorkflowId = new Map<string, string>();
    for (const snapshot of Object.values(kanbanSnapshots)) {
      for (const task of snapshot.tasks) tasks.set(task.id, task);
    }
    for (const [workflowId, snapshot] of Object.entries(kanbanSnapshots)) {
      const lastStepId = findLastWorkflowStepId(snapshot.steps);
      if (lastStepId) lastStepIdByWorkflowId.set(workflowId, lastStepId);
    }
    for (const task of kanbanTasks) tasks.set(task.id, task);
    const activeLastStepId = findLastWorkflowStepId(kanbanSteps);
    if (kanbanWorkflowId && activeLastStepId) {
      lastStepIdByWorkflowId.set(kanbanWorkflowId, activeLastStepId);
    }
    return { tasks, lastStepIdByWorkflowId, steps: kanbanSteps };
  }, [kanbanSnapshots, kanbanSteps, kanbanTasks, kanbanWorkflowId]);
}

function useCommandPanelRepositories(workspaceId: string | null) {
  const repositories = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  return workspaceId ? (repositories[workspaceId] ?? []) : [];
}

// eslint-disable-next-line max-lines-per-function -- composition root keeps hook ordering and view wiring together.
export function CommandPanel() {
  const { open, setOpen, mode: panelMode, setMode, modeRequestVersion } = useCommandPanelOpen();
  const commands = useCommands();
  const pathname = usePathname();
  const {
    tasks: liveTasksById,
    lastStepIdByWorkflowId,
    steps: kanbanSteps,
  } = useCommandPanelLiveTasks();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const workspaceSearchAvailable = useAppStore((state) =>
    isTaskWorkspaceSearchAvailable(state, pathname),
  );
  const worktreePath = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId]?.worktree_path ?? null) : null,
  );
  const repositories = useCommandPanelRepositories(workspaceId);
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
      liveTasksById={liveTasksById}
      lastStepIdByWorkflowId={lastStepIdByWorkflowId}
      handleTaskSelect={handlers.handleTaskSelect}
    />
  );
}
