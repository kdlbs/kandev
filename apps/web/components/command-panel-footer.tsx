"use client";

import { useEffect, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandInput,
  CommandList,
} from "@kandev/ui/command";
import { Kbd, KbdGroup } from "@kandev/ui/kbd";
import type { CommandPanelMode, CommandItem as CommandItemType } from "@/lib/commands/types";
import type { Task } from "@/lib/types/http";
import type { FileSearchResult } from "@/lib/types/backend";
import { WorkspaceContentSearch } from "@/components/workspace-content-search";
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
import {
  CommandsListContent,
  FileSearchContent,
  MODE_COMMANDS,
  MODE_SEARCH_CONTENT,
  MODE_SEARCH_FILES,
  MODE_SEARCH_TASKS,
  TaskSearchContent,
  type StepMap,
} from "@/components/command-panel-results";

export {
  MODE_COMMANDS,
  MODE_SEARCH_CONTENT,
  MODE_SEARCH_FILES,
  MODE_SEARCH_TASKS,
  getFileResultValue,
  getTaskResultValue,
} from "@/components/command-panel-results";

// `mode` stays an untranslated discriminant — it is compared with `===`
// throughout. Only the labels it selects are copy.
function getInputPlaceholder(
  t: TFunction,
  mode: CommandPanelMode,
  inputCommand: CommandItemType | null,
) {
  if (mode === "input") return inputCommand?.inputPlaceholder ?? t("common:enterValue");
  if (mode === MODE_SEARCH_TASKS) return t("common:searchForTasks");
  if (mode === MODE_SEARCH_FILES) return t("common:searchForFiles");
  if (mode === MODE_SEARCH_CONTENT) return t("common:searchTaskContents");
  return t("common:typeACommand");
}

function getEnterLabel(t: TFunction, mode: CommandPanelMode) {
  if (mode === "input") return t("common:confirm");
  if (mode === MODE_SEARCH_TASKS || mode === MODE_SEARCH_FILES || mode === MODE_SEARCH_CONTENT) {
    return t("common:open");
  }
  return t("common:select");
}

function getModeLabel(t: TFunction, mode: CommandPanelMode, inputCommand: CommandItemType | null) {
  if (mode === "input") return inputCommand?.label;
  if (mode === MODE_SEARCH_TASKS) return t("common:tasks");
  if (mode === MODE_SEARCH_FILES) return t("common:files");
  if (mode === MODE_SEARCH_CONTENT) return t("common:contents");
  return null;
}

function CommandPanelFooter({ mode }: { mode: CommandPanelMode }) {
  const { t } = useTranslation();
  const isScopeMode = isCommandPanelScopeMode(mode);
  return (
    <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
      {isScopeMode && (
        <>
          <KbdGroup>
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>{t("common:navigate")}</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>Tab</Kbd>
            <span>{t("common:switchMode")}</span>
          </KbdGroup>
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
  stepMap: StepMap;
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
  const modeLabel = getModeLabel(t, mode, inputCommand);
  const onInputKeyDown = (event: React.KeyboardEvent) => {
    if (
      isTopLevelMode &&
      event.key === "Tab" &&
      !event.altKey &&
      !event.ctrlKey &&
      !event.metaKey
    ) {
      event.preventDefault();
      onScopeChange(getAdjacentCommandPanelScope(mode, event.shiftKey, workspaceSearchAvailable));
      return;
    }
    handleKeyDown(event);
  };
  return (
    // Four scope tabs do not fit beside the input on a phone, and the tablist
    // does not shrink, so the input would collapse to a few characters. The
    // min-width forces the tablist onto its own row instead of squeezing the
    // query out of view.
    <div className="flex min-h-10 flex-wrap items-center border-b border-border [&>[data-slot=command-input-wrapper]]:min-w-44 [&>[data-slot=command-input-wrapper]]:flex-1 [&>[data-slot=command-input-wrapper]]:pb-1">
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
      {isTopLevelMode && (
        <CommandPanelScopeSwitcher
          mode={mode}
          onScopeChange={onScopeChange}
          workspaceSearchAvailable={workspaceSearchAvailable}
        />
      )}
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
      {mode === MODE_SEARCH_TASKS && (
        <TaskSearchContent
          tasks={taskResults}
          isSearching={isSearching}
          search={search}
          stepMap={stepMap}
          repoMap={repoMap}
          onSelect={handleTaskSelect}
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
        <CommandPanelFooter mode={renderedProps.mode} />
      </Command>
    </CommandDialog>
  );
}
