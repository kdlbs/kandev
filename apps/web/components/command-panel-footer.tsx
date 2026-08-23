"use client";

import { useEffect, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatTimeDistance, useDateLocale } from "@/lib/i18n/date-locale";
import { IconArchive, IconArrowRight, IconHammer, IconLoader2 } from "@tabler/icons-react";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@kandev/ui/command";
import { Kbd, KbdGroup } from "@kandev/ui/kbd";
import { Badge } from "@kandev/ui/badge";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import {
  getCommandSearchTerms,
  scoreCommandSearch,
  sortCommandsForSearch,
} from "@/lib/commands/search";
import { formatShortcut } from "@/lib/keyboard/utils";
import type { Task } from "@/lib/types/http";
import type { FileSearchResult } from "@/lib/types/backend";
import { FileIcon } from "@/components/ui/file-icon";
import { WorkspaceContentSearch } from "@/components/workspace-content-search";
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { groupByRepositoryName, isSingleRepoGroup } from "@/lib/group-by-repo";
import {
  CommandPanelScopeSwitcher,
  getAdjacentCommandPanelScope,
  isCommandPanelScopeMode,
  type CommandPanelScopeMode,
} from "@/components/command-panel-scope-switcher";
import type { WorkspaceContentSearchError } from "@/hooks/domains/session/use-workspace-content-search";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import {
  CommandPanelConfirmation,
  dismissCommandConfirmation,
  getCommandConfirmationState,
} from "@/components/command-panel-confirmation";

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);
export const MODE_COMMANDS: CommandPanelMode = "commands";
export const MODE_SEARCH_FILES: CommandPanelMode = "search-files";
export const MODE_SEARCH_CONTENT: CommandPanelMode = "search-content";

const STEP_COLOR_MAP: Record<string, string> = {
  "bg-slate-500": "#64748b",
  "bg-red-500": "#ef4444",
  "bg-orange-500": "#f97316",
  "bg-yellow-500": "#eab308",
  "bg-green-500": "#22c55e",
  "bg-cyan-500": "#06b6d4",
  "bg-blue-500": "#3b82f6",
  "bg-indigo-500": "#6366f1",
  "bg-purple-500": "#a855f7",
};

function getFileName(filePath: string) {
  return filePath.split("/").pop() ?? filePath;
}

export function getTaskResultValue(task: Task) {
  return `__task:${task.id} ${task.title}`;
}

export function getFileResultValue(filePath: string) {
  return `__file:${filePath}`;
}

function CommandItemRow({
  cmd,
  onSelect,
}: {
  cmd: CommandItemType;
  onSelect: (cmd: CommandItemType) => void;
}) {
  return (
    <CommandItem
      key={cmd.id}
      value={cmd.id}
      keywords={getCommandSearchTerms(cmd)}
      onSelect={() => onSelect(cmd)}
    >
      {cmd.icon && (
        <span
          className={
            cmd.context ? "self-start pt-0.5 text-muted-foreground" : "text-muted-foreground"
          }
        >
          {cmd.icon}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate">{cmd.label}</span>
        {cmd.context && (
          <span className="block truncate text-[0.625rem] leading-4 text-muted-foreground">
            {cmd.context}
          </span>
        )}
      </span>
      {cmd.shortcut && <CommandShortcut>{formatShortcut(cmd.shortcut)}</CommandShortcut>}
      {cmd.enterMode && (
        <span className="ml-auto text-muted-foreground">
          <IconArrowRight className="size-3" />
        </span>
      )}
    </CommandItem>
  );
}

type TaskResultItemProps = {
  task: Task;
  stepMap: Map<string, { name: string; color: string }>;
  repoMap: Map<string, string>;
  onSelect: (task: Task) => void;
};

function TaskResultItem({ task, stepMap, repoMap, onSelect }: TaskResultItemProps) {
  const locale = useDateLocale();
  const isArchived = ARCHIVED_STATES.has(task.state);
  const step = stepMap.get(task.workflow_step_id);
  const stepHex = step ? STEP_COLOR_MAP[step.color] : undefined;
  const rawPath =
    task.primary_working_directory ??
    (task.repositories?.[0] ? repoMap.get(task.repositories[0].repository_id) : undefined);
  const workDir = rawPath ? getFileName(rawPath) : undefined;
  const details: string[] = [];
  if (workDir) details.push(workDir);
  if (task.primary_agent_name) details.push(task.primary_agent_name);
  if (task.updated_at) {
    details.push(formatTimeDistance(task.updated_at, locale));
  }
  return (
    <CommandItem
      key={task.id}
      value={getTaskResultValue(task)}
      onSelect={() => onSelect(task)}
      className={isArchived ? "opacity-60" : ""}
      forceMount
    >
      <div className="flex items-center gap-2 min-w-0 w-full">
        {isArchived ? (
          <IconArchive className="size-3 shrink-0 text-muted-foreground" />
        ) : (
          <IconHammer className="size-3 shrink-0 text-muted-foreground" />
        )}
        <span className="truncate font-medium">{task.title}</span>
        {step && (
          <Badge
            variant="secondary"
            className="text-[0.6rem] shrink-0"
            style={stepHex ? { backgroundColor: stepHex + "22", color: stepHex } : undefined}
          >
            {step.name}
          </Badge>
        )}
        {details.length > 0 && (
          <span className="ml-auto text-[0.6rem] text-muted-foreground truncate shrink-0">
            {details.join(" · ")}
          </span>
        )}
      </div>
    </CommandItem>
  );
}

type CommandsListContentProps = {
  commands: CommandItemType[];
  grouped: [string, CommandItemType[]][];
  search: string;
  onSelect: (cmd: CommandItemType) => void;
  taskResults: Task[];
  isSearching: boolean;
  stepMap: Map<string, { name: string; color: string }>;
  repoMap: Map<string, string>;
  onTaskSelect: (task: Task) => void;
};

function SearchCommandGroups({
  commands,
  search,
  onSelect,
}: Pick<CommandsListContentProps, "commands" | "search" | "onSelect">) {
  const { t } = useTranslation();
  const matches = sortCommandsForSearch(commands, search).filter(
    (cmd) => scoreCommandSearch(cmd.id, search, getCommandSearchTerms(cmd)) > 0,
  );
  const regularMatches = matches.filter((cmd) => !cmd.searchOnly);
  const searchOnlyGroups = new Map<string, CommandItemType[]>();

  for (const cmd of matches) {
    if (!cmd.searchOnly) continue;
    const groupItems = searchOnlyGroups.get(cmd.group) ?? [];
    groupItems.push(cmd);
    searchOnlyGroups.set(cmd.group, groupItems);
  }

  return (
    <>
      {regularMatches.length > 0 && (
        <CommandGroup heading={t("common:commandGroupCommands")}>
          {regularMatches.map((cmd) => (
            <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
          ))}
        </CommandGroup>
      )}
      {[...searchOnlyGroups].map(([group, items]) => (
        <CommandGroup key={group} heading={group}>
          {items.map((cmd) => (
            <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
          ))}
        </CommandGroup>
      ))}
    </>
  );
}

function CommandsListContent({
  commands,
  grouped,
  search,
  onSelect,
  taskResults,
  isSearching,
  stepMap,
  repoMap,
  onTaskSelect,
}: CommandsListContentProps) {
  const { t } = useTranslation();
  const hasInlineResults = taskResults.length > 0 || isSearching;
  return (
    <>
      {!hasInlineResults && !isSearching && (
        <CommandEmpty>{t("common:noCommandsFound")}</CommandEmpty>
      )}
      {isSearching && taskResults.length === 0 && (
        <CommandGroup heading={t("common:activeTasks")} forceMount>
          <div className="flex items-center justify-center py-3">
            <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
          </div>
        </CommandGroup>
      )}
      {taskResults.length > 0 && (
        <CommandGroup
          heading={search.trim() ? t("common:tasks") : t("common:activeTasks")}
          forceMount
        >
          {taskResults.map((task) => (
            <TaskResultItem
              key={task.id}
              task={task}
              stepMap={stepMap}
              repoMap={repoMap}
              onSelect={onTaskSelect}
            />
          ))}
        </CommandGroup>
      )}
      {search.trim() ? (
        <SearchCommandGroups commands={commands} search={search} onSelect={onSelect} />
      ) : (
        grouped.map(([group, items]) => {
          const visibleItems = items.filter((cmd) => !cmd.searchOnly);
          if (visibleItems.length === 0) return null;
          return (
            <CommandGroup key={group} heading={group}>
              {visibleItems.map((cmd) => (
                <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
              ))}
            </CommandGroup>
          );
        })
      )}
    </>
  );
}

type FileSearchContentProps = {
  files: FileSearchResult[];
  isSearching: boolean;
  search: string;
  sessionId: string | null;
  onSelect: (path: string) => void;
};

function FileSearchContent({
  files,
  isSearching,
  search,
  sessionId,
  onSelect,
}: FileSearchContentProps) {
  const { t } = useTranslation();
  const getRepoDisplayName = useRepoDisplayName(sessionId);
  if (isSearching && files.length === 0) {
    return (
      <div className="flex items-center justify-center py-6">
        <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (search.trim() && files.length === 0)
    return <CommandEmpty>{t("common:noFilesFound")}</CommandEmpty>;
  if (!search.trim()) return <CommandEmpty>{t("common:typeToSearchFiles")}</CommandEmpty>;

  const groups = groupByRepositoryName(files, (file) => file.repository_name);
  const singleRepo = isSingleRepoGroup(groups);
  return groups.map((group) => (
    <CommandGroup
      key={group.repositoryName || "workspace"}
      heading={
        singleRepo
          ? t("common:files")
          : (getRepoDisplayName(group.repositoryName) ??
            group.repositoryName ??
            t("common:workspace"))
      }
      forceMount
      data-testid="file-search-repo-group"
      data-repository={group.repositoryName}
    >
      {group.items.map((file) => {
        const repositoryPrefix = file.repository_name ? `${file.repository_name}/` : "";
        const displayPath = file.path.startsWith(repositoryPrefix)
          ? file.path.slice(repositoryPrefix.length)
          : file.path;
        const fileName = getFileName(displayPath);
        const lastSlash = displayPath.lastIndexOf("/");
        const dir = lastSlash > 0 ? displayPath.slice(0, lastSlash) : "";
        return (
          <CommandItem
            key={file.path}
            value={getFileResultValue(file.path)}
            onSelect={() => onSelect(file.path)}
            forceMount
          >
            <FileIcon fileName={fileName} className="shrink-0" />
            <span className="truncate font-medium">{fileName}</span>
            {dir && <span className="ml-1 truncate text-xs text-muted-foreground">{dir}</span>}
          </CommandItem>
        );
      })}
    </CommandGroup>
  ));
}

// `mode` stays an untranslated discriminant — it is compared with `===`
// throughout. Only the labels it selects are copy.
function getInputPlaceholder(
  t: TFunction,
  mode: CommandPanelMode,
  inputCommand: CommandItemType | null,
) {
  if (mode === "input") return inputCommand?.inputPlaceholder ?? t("common:enterValue");
  if (mode === "search-tasks") return t("common:searchForTasks");
  if (mode === MODE_SEARCH_FILES) return t("common:searchForFiles");
  if (mode === MODE_SEARCH_CONTENT) return t("common:searchTaskContents");
  return t("common:typeACommand");
}

function getEnterLabel(t: TFunction, mode: CommandPanelMode) {
  if (mode === "input") return t("common:confirm");
  if (mode === "search-tasks" || mode === MODE_SEARCH_FILES || mode === MODE_SEARCH_CONTENT) {
    return t("common:open");
  }
  return t("common:select");
}

function getModeLabel(t: TFunction, mode: CommandPanelMode, inputCommand: CommandItemType | null) {
  if (mode === "input") return inputCommand?.label;
  if (mode === "search-tasks") return t("common:tasks");
  if (mode === MODE_SEARCH_FILES) return t("common:files");
  if (mode === MODE_SEARCH_CONTENT) return t("common:contents");
  return null;
}

function CommandPanelFooter({
  mode,
  workspaceSearchAvailable,
}: {
  mode: CommandPanelMode;
  workspaceSearchAvailable: boolean;
}) {
  const { t } = useTranslation();
  const isScopeMode = isCommandPanelScopeMode(mode);
  const canSwitchScope = workspaceSearchAvailable && isScopeMode;
  return (
    <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
      {isScopeMode && (
        <>
          <KbdGroup>
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>{t("common:navigate")}</span>
          </KbdGroup>
          {canSwitchScope && (
            <KbdGroup>
              <Kbd>Tab</Kbd>
              <span>{t("common:switchMode")}</span>
            </KbdGroup>
          )}
        </>
      )}
      <KbdGroup>
        <Kbd>↵</Kbd>
        <span>{getEnterLabel(t, mode)}</span>
      </KbdGroup>
      {!isScopeMode && (
        <KbdGroup>
          <Kbd>⌫</Kbd>
          <span>{t("common:back")}</span>
        </KbdGroup>
      )}
      <KbdGroup>
        {/* A key name, not copy — it labels the physical key. */}
        <Kbd>esc</Kbd>
        <span>{t("common:close")}</span>
      </KbdGroup>
    </div>
  );
}

export type CommandPanelViewProps = {
  open: boolean;
  setOpen: (open: boolean) => void;
  mode: CommandPanelMode;
  inputCommand: CommandItemType | null;
  selectedValue: string;
  setSelectedValue: Dispatch<SetStateAction<string>>;
  search: string;
  setSearch: (value: string) => void;
  handleKeyDown: (e: React.KeyboardEvent) => void;
  onScopeChange: (mode: CommandPanelScopeMode) => void;
  goBack: () => void;
  fileResults: FileSearchResult[];
  isSearchingFiles: boolean;
  handleFileSelect: (filePath: string) => void;
  contentResults: WorkspaceContentSearchResult[];
  isSearchingContent: boolean;
  contentSearchError: WorkspaceContentSearchError | null;
  activeSessionId: string | null;
  workspaceSearchAvailable: boolean;
  handleContentSelect: (result: WorkspaceContentSearchResult) => void;
  commands: CommandItemType[];
  grouped: Array<[string, CommandItemType[]]>;
  handleSelect: (cmd: CommandItemType) => void;
  isSearching: boolean;
  taskResults: Task[];
  stepMap: Map<string, { name: string; color: string }>;
  repoMap: Map<string, string>;
  handleTaskSelect: (task: Task) => void;
};

function CommandPanelInputHeader({
  mode,
  inputCommand,
  search,
  setSearch,
  handleKeyDown,
  onScopeChange,
  goBack,
  workspaceSearchAvailable,
}: CommandPanelViewProps) {
  const { t } = useTranslation();
  const isTopLevelMode = isCommandPanelScopeMode(mode);
  const canSwitchScope = workspaceSearchAvailable && isTopLevelMode;
  const modeLabel = getModeLabel(t, mode, inputCommand);
  const onInputKeyDown = (event: React.KeyboardEvent) => {
    if (
      canSwitchScope &&
      event.key === "Tab" &&
      !event.altKey &&
      !event.ctrlKey &&
      !event.metaKey
    ) {
      event.preventDefault();
      onScopeChange(getAdjacentCommandPanelScope(mode, event.shiftKey));
      return;
    }
    handleKeyDown(event);
  };
  return (
    <div className="flex min-h-10 items-center border-b border-border [&>[data-slot=command-input-wrapper]]:min-w-0 [&>[data-slot=command-input-wrapper]]:flex-1 [&>[data-slot=command-input-wrapper]]:pb-1">
      {!isTopLevelMode && (
        <button
          onClick={goBack}
          tabIndex={-1}
          className="shrink-0 pl-2 flex min-h-10 cursor-pointer items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          <span>←</span>
          <span>{modeLabel}</span>
          <span className="text-muted-foreground/50">›</span>
        </button>
      )}
      <CommandInput
        placeholder={getInputPlaceholder(t, mode, inputCommand)}
        value={search}
        onValueChange={setSearch}
        onKeyDown={onInputKeyDown}
      />
      {canSwitchScope && <CommandPanelScopeSwitcher mode={mode} onScopeChange={onScopeChange} />}
    </div>
  );
}

function CommandPanelResultList(props: CommandPanelViewProps) {
  const { t } = useTranslation();
  const {
    mode,
    inputCommand,
    search,
    fileResults,
    isSearchingFiles,
    handleFileSelect,
    contentResults,
    isSearchingContent,
    contentSearchError,
    activeSessionId,
    handleContentSelect,
    commands,
    grouped,
    handleSelect,
    isSearching,
    taskResults,
    stepMap,
    repoMap,
    handleTaskSelect,
  } = props;
  const { confirmationCommand, visibleCommands, visibleGroups } = getCommandConfirmationState(
    commands,
    grouped,
  );
  return (
    <CommandList>
      {confirmationCommand && <CommandPanelConfirmation command={confirmationCommand} />}
      {mode === MODE_COMMANDS && (
        <CommandsListContent
          commands={visibleCommands}
          grouped={visibleGroups}
          search={search}
          onSelect={handleSelect}
          taskResults={taskResults}
          isSearching={isSearching}
          stepMap={stepMap}
          repoMap={repoMap}
          onTaskSelect={handleTaskSelect}
        />
      )}
      {mode === MODE_SEARCH_FILES && (
        <FileSearchContent
          files={fileResults}
          isSearching={isSearchingFiles}
          search={search}
          sessionId={activeSessionId}
          onSelect={handleFileSelect}
        />
      )}
      {mode === MODE_SEARCH_CONTENT && (
        <WorkspaceContentSearch
          results={contentResults}
          isSearching={isSearchingContent}
          error={contentSearchError}
          search={search}
          sessionId={activeSessionId}
          onSelect={handleContentSelect}
        />
      )}
      {mode === "input" &&
        (!search.trim() ? (
          <CommandEmpty>{inputCommand?.inputPlaceholder ?? t("common:enterAValue")}</CommandEmpty>
        ) : (
          <CommandEmpty>{t("common:pressEnterToConfirm")}</CommandEmpty>
        ))}
    </CommandList>
  );
}

export function CommandPanelView(props: CommandPanelViewProps) {
  const {
    open,
    setOpen,
    mode,
    selectedValue,
    setSelectedValue,
    workspaceSearchAvailable,
    onScopeChange,
  } = props;
  const workspaceModeUnavailable =
    !workspaceSearchAvailable && (mode === MODE_SEARCH_FILES || mode === MODE_SEARCH_CONTENT);
  const renderedProps = workspaceModeUnavailable ? { ...props, mode: MODE_COMMANDS } : props;

  useEffect(() => {
    if (workspaceModeUnavailable) onScopeChange("commands");
  }, [onScopeChange, workspaceModeUnavailable]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) dismissCommandConfirmation(props.commands);
    setOpen(nextOpen);
  };

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      overlayClassName="supports-backdrop-filter:backdrop-blur-none!"
    >
      <Command
        // Mode changes replace whole group trees. Filtering before render avoids
        // cmdk's deferred sorter retaining a group after that group unmounts.
        shouldFilter={false}
        loop
        // cmdk's built-in vim bindings intercept Ctrl/Cmd+K, +P, +J, +N as
        // list-navigation shortcuts and call `event.preventDefault()` on the
        // React (bubble) dispatch, which runs before this event reaches our
        // `window`-level `useKeyboardShortcut` listeners. That collided with
        // the app's own COMMAND_PANEL-open (Ctrl/Cmd+K) and COMMAND_PANEL
        // (Ctrl/Cmd+P) shortcuts: once `useKeyboardShortcut` started bailing
        // on `event.defaultPrevented` (see that hook's core-vs-plugin
        // precedence guard), cmdk's vim-binding preventDefault silently
        // suppressed the panel's own close/reopen toggle. Disable vim
        // bindings here so this palette's Ctrl/Cmd+K and +P always reach the
        // app-level toggle instead of cmdk's internal navigation.
        vimBindings={false}
        value={selectedValue}
        onValueChange={setSelectedValue}
      >
        <CommandPanelInputHeader {...renderedProps} />
        <CommandPanelResultList {...renderedProps} />
        <CommandPanelFooter
          mode={renderedProps.mode}
          workspaceSearchAvailable={workspaceSearchAvailable}
        />
      </Command>
    </CommandDialog>
  );
}
