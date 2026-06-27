"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@/lib/routing/client-router";
import { useCommands, useCommandPanelOpen } from "@/lib/commands/command-registry";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useAppStore } from "@/components/state-provider";
import { useCachedRepositories } from "@/hooks/domains/workspace/use-repository-cache";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";

import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useDebounce } from "@/hooks/use-debounce";
import { workspaceTasksQueryOptions } from "@/lib/query/query-options";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import {
  CommandPanelView,
  MODE_COMMANDS,
  MODE_SEARCH_FILES,
  getFileResultValue,
  getTaskResultValue,
} from "@/components/command-panel-footer";

function getFileName(filePath: string) {
  return filePath.split("/").pop() ?? filePath;
}

function useCommandPanelState() {
  const [mode, setMode] = useState<CommandPanelMode>(MODE_COMMANDS);
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
  const fileDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  return { fileDebounceRef };
}

type FileSearchEffectOptions = {
  mode: CommandPanelMode;
  search: string;
  activeSessionId: string | null;
  setFileResults: (files: string[]) => void;
  setIsSearchingFiles: (searching: boolean) => void;
  fileDebounceRef: React.RefObject<ReturnType<typeof setTimeout> | null>;
};

function useFileSearchEffect(opts: FileSearchEffectOptions) {
  const { mode, search, activeSessionId, setFileResults, setIsSearchingFiles, fileDebounceRef } =
    opts;
  useEffect(() => {
    if (mode !== MODE_SEARCH_FILES || !search.trim() || !activeSessionId) {
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
  }, [activeSessionId, fileDebounceRef, mode, search, setFileResults, setIsSearchingFiles]);
}

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);

type CommandPanelStep = {
  id: string;
  title: string;
  color: string;
  position: number;
  show_in_command_panel?: boolean;
};

function resolveVisibleStepIds(steps: CommandPanelStep[]) {
  if (steps.length === 0) return null; // no steps loaded yet — don't filter
  return new Set(steps.filter((s) => s.show_in_command_panel !== false).map((s) => s.id));
}

type InlineTaskSearchOptions = {
  mode: CommandPanelMode;
  search: string;
  open: boolean;
  workspaceId: string | null;
  steps: CommandPanelStep[];
  setTaskResults: (tasks: Task[]) => void;
  setIsSearching: (searching: boolean) => void;
};

function useStepMaps(steps: CommandPanelStep[]) {
  const visibleStepIds = useMemo(() => resolveVisibleStepIds(steps), [steps]);
  const stepPositionMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const step of steps) map.set(step.id, step.position);
    return map;
  }, [steps]);
  return { visibleStepIds, stepPositionMap };
}

export function getCommandPanelActiveTaskResults(
  tasks: Task[],
  visibleStepIds: ReadonlySet<string> | null,
  stepPositionMap: ReadonlyMap<string, number>,
) {
  return tasks
    .filter(
      (task) =>
        (!visibleStepIds || visibleStepIds.has(task.workflow_step_id)) &&
        !ARCHIVED_STATES.has(task.state),
    )
    .sort(
      (a, b) =>
        (stepPositionMap.get(a.workflow_step_id) ?? 99) -
        (stepPositionMap.get(b.workflow_step_id) ?? 99),
    );
}

export function getCommandPanelSearchTaskResults(tasks: Task[]) {
  return [...tasks].sort((a, b) => {
    const aArchived = ARCHIVED_STATES.has(a.state) ? 1 : 0;
    const bArchived = ARCHIVED_STATES.has(b.state) ? 1 : 0;
    return aArchived - bArchived;
  });
}

function useInlineTaskSearchEffect(opts: InlineTaskSearchOptions) {
  const { mode, search, open, workspaceId, steps, setTaskResults, setIsSearching } = opts;
  const { visibleStepIds, stepPositionMap } = useStepMaps(steps);
  const trimmedSearch = search.trim();
  const debouncedSearch = useDebounce(trimmedSearch, 300);
  const isBlankSearch = trimmedSearch.length === 0;
  const isShortSearch = trimmedSearch.length > 0 && trimmedSearch.length < 2;
  const isDebouncingSearch = trimmedSearch.length >= 2 && debouncedSearch !== trimmedSearch;
  const queryFilters = isBlankSearch
    ? { page: 1, pageSize: 20 }
    : { query: debouncedSearch, page: 1, pageSize: 5, includeArchived: true };
  const taskQuery = useQuery({
    ...workspaceTasksQueryOptions(workspaceId ?? "", queryFilters),
    enabled:
      mode === MODE_COMMANDS &&
      open &&
      Boolean(workspaceId) &&
      !isShortSearch &&
      !isDebouncingSearch,
    staleTime: 0,
  });

  useEffect(() => {
    if (mode !== MODE_COMMANDS) {
      return;
    }
    if (!open || !workspaceId || isShortSearch) {
      setTaskResults([]);
      setIsSearching(false);
      return;
    }
    if (isDebouncingSearch) {
      setIsSearching(true);
      return;
    }
    setIsSearching(taskQuery.isFetching);
  }, [
    isDebouncingSearch,
    isShortSearch,
    mode,
    open,
    setIsSearching,
    setTaskResults,
    taskQuery.isFetching,
    workspaceId,
  ]);

  useEffect(() => {
    if (mode !== MODE_COMMANDS || !open || !workspaceId || !taskQuery.data) return;
    const tasks = taskQuery.data.tasks ?? [];
    setTaskResults(
      isBlankSearch
        ? getCommandPanelActiveTaskResults(tasks, visibleStepIds, stepPositionMap)
        : getCommandPanelSearchTaskResults(tasks),
    );
  }, [
    isBlankSearch,
    mode,
    open,
    setTaskResults,
    stepPositionMap,
    taskQuery.data,
    visibleStepIds,
    workspaceId,
  ]);

  useEffect(() => {
    if (!taskQuery.isError) return;
    setTaskResults([]);
    setIsSearching(false);
  }, [setIsSearching, setTaskResults, taskQuery.isError]);
}

function useCommandPanelEffects(
  open: boolean,
  state: ReturnType<typeof useCommandPanelState>,
  workspaceId: string | null,
  activeSessionId: string | null,
  steps: CommandPanelStep[],
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
    activeSessionId,
    setFileResults,
    setIsSearchingFiles,
    fileDebounceRef,
  });
}

function useFirstResultSelection(open: boolean, state: ReturnType<typeof useCommandPanelState>) {
  const { mode, search, taskResults, fileResults, setSelectedValue } = state;

  // `search` intentionally re-applies the first result while debounced results are loading.
  useEffect(() => {
    if (!open) return;

    if (mode === MODE_COMMANDS) {
      const firstTask = taskResults[0];
      if (firstTask) {
        setSelectedValue(getTaskResultValue(firstTask));
        return;
      }
      setSelectedValue((current) => (current.startsWith("__task:") ? "" : current));
      return;
    }

    if (mode === MODE_SEARCH_FILES) {
      const firstFile = fileResults[0];
      setSelectedValue(firstFile ? getFileResultValue(firstFile) : "");
      return;
    }

    setSelectedValue("");
  }, [fileResults, mode, open, search, setSelectedValue, taskResults]);
}

function useCommandPanelHandlers(
  state: ReturnType<typeof useCommandPanelState>,
  setOpen: (open: boolean) => void,
  commands: CommandItemType[],
  steps: CommandPanelStep[],
  repositories: Array<{ id: string; local_path: string }>,
) {
  const { mode, search, inputCommand, setMode, setSearch, setInputCommand } = state;
  const router = useRouter();

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
    for (const step of steps) map.set(step.id, { name: step.title, color: step.color });
    return map;
  }, [steps]);

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
      router.push(linkToTask(task.id));
    },
    [setOpen, router],
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
      if (mode !== MODE_COMMANDS && e.key === "Backspace" && !search) {
        e.preventDefault();
        setMode(MODE_COMMANDS);
        setSearch("");
        setInputCommand(null);
      }
    },
    [mode, search, inputCommand, setOpen, setMode, setSearch, setInputCommand],
  );

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
    goBack,
  };
}

export function getCommandPanelStepsFromSnapshots(
  snapshots: Record<string, WorkflowSnapshotData>,
): CommandPanelStep[] {
  return Object.values(snapshots).flatMap((snapshot) =>
    snapshot.steps.map((step) => ({
      id: step.id,
      title: step.title,
      color: step.color,
      position: step.position,
      show_in_command_panel: step.show_in_command_panel,
    })),
  );
}

function getCommandPanelStepsSignature(snapshots: Record<string, WorkflowSnapshotData>): string {
  return Object.values(snapshots)
    .flatMap((snapshot) =>
      snapshot.steps.map(
        (step) =>
          `${snapshot.workflowId}:${step.id}:${step.title}:${step.color}:${step.position}:${step.show_in_command_panel}`,
      ),
    )
    .sort()
    .join("|");
}

function useCommandPanelSteps(workspaceId: string | null): CommandPanelStep[] {
  const { snapshots } = useAllWorkflowSnapshots(workspaceId);
  const stableStepsRef = useRef<{ signature: string; steps: CommandPanelStep[] }>({
    signature: "",
    steps: [],
  });
  const signature = getCommandPanelStepsSignature(snapshots);
  if (stableStepsRef.current.signature !== signature) {
    stableStepsRef.current = {
      signature,
      steps: getCommandPanelStepsFromSnapshots(snapshots),
    };
  }
  return stableStepsRef.current.steps;
}

export function CommandPanel() {
  const { open, setOpen } = useCommandPanelOpen();
  const commands = useCommands();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const repositories = useCachedRepositories(workspaceId);
  const steps = useCommandPanelSteps(workspaceId);

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

  useCommandPanelEffects(open, state, workspaceId, activeSessionId, steps);
  useFirstResultSelection(open, state);

  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);

  const toggleCommands = useCallback(() => setOpen(!openRef.current), [setOpen]);

  const openFileSearch = useCallback(() => {
    if (openRef.current && state.mode === MODE_SEARCH_FILES) {
      setOpen(false);
    } else {
      state.setMode(MODE_SEARCH_FILES);
      state.setSearch("");
      setOpen(true);
    }
  }, [setOpen, state]);

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const searchShortcut = getShortcut("SEARCH", keyboardShortcuts);
  const fileSearchShortcut = getShortcut("FILE_SEARCH", keyboardShortcuts);

  const commandPanelShortcut = getShortcut("COMMAND_PANEL", keyboardShortcuts);
  useKeyboardShortcut(searchShortcut, toggleCommands);
  useKeyboardShortcut(commandPanelShortcut, toggleCommands);
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
  } = useCommandPanelHandlers(state, setOpen, commands, steps, repositories);

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
