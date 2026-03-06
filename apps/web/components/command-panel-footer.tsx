"use client";

import { formatDistanceToNow } from "date-fns";
import { IconArrowRight, IconLoader2, IconSearch } from "@tabler/icons-react";
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
import { formatShortcut } from "@/lib/keyboard/utils";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useAppStore } from "@/components/state-provider";
import type { Task } from "@/lib/types/http";
import { FileIcon } from "@/components/ui/file-icon";

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);
const MODE_COMMANDS: CommandPanelMode = "commands";
const MODE_SEARCH_FILES: CommandPanelMode = "search-files";

function getFileName(filePath: string) {
  return filePath.split("/").pop() ?? filePath;
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
      value={cmd.id + " " + cmd.label + " " + (cmd.keywords?.join(" ") ?? "")}
      onSelect={() => onSelect(cmd)}
    >
      {cmd.icon && <span className="text-muted-foreground">{cmd.icon}</span>}
      <span>{cmd.label}</span>
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
  onSelect: (task: Task) => void;
};

function TaskResultItem({ task, stepMap, onSelect }: TaskResultItemProps) {
  const isArchived = ARCHIVED_STATES.has(task.state);
  const step = stepMap.get(task.workflow_step_id);
  return (
    <CommandItem
      key={task.id}
      value={`__task:${task.id} ${task.title}`}
      onSelect={() => onSelect(task)}
      className={isArchived ? "opacity-60" : ""}
      forceMount
    >
      <div className="flex flex-col gap-0.5 min-w-0">
        <div className="flex items-center gap-2">
          <IconSearch className="size-3 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium">{task.title}</span>
          {step && (
            <Badge variant="secondary" className="text-[0.6rem] shrink-0">
              {step.name}
            </Badge>
          )}
          {isArchived && (
            <Badge variant="outline" className="text-[0.6rem] shrink-0 opacity-70">
              {task.state}
            </Badge>
          )}
        </div>
        {task.updated_at && (
          <div className="flex items-center gap-1.5 text-[0.6rem] text-muted-foreground pl-5">
            <span>{formatDistanceToNow(new Date(task.updated_at), { addSuffix: true })}</span>
          </div>
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
  onTaskSelect: (task: Task) => void;
};

function CommandsListContent({
  commands,
  grouped,
  search,
  onSelect,
  taskResults,
  isSearching,
  stepMap,
  onTaskSelect,
}: CommandsListContentProps) {
  const hasInlineResults = taskResults.length > 0 || isSearching;
  return (
    <>
      {!hasInlineResults && <CommandEmpty>No commands found.</CommandEmpty>}
      {search.trim() ? (
        <CommandGroup>
          {commands.map((cmd) => (
            <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
          ))}
        </CommandGroup>
      ) : (
        grouped.map(([group, items]) => (
          <CommandGroup key={group} heading={group}>
            {items.map((cmd) => (
              <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
            ))}
          </CommandGroup>
        ))
      )}
      {search.trim() && isSearching && taskResults.length === 0 && (
        <CommandGroup heading="Tasks" forceMount>
          <div className="flex items-center justify-center py-3">
            <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
          </div>
        </CommandGroup>
      )}
      {taskResults.length > 0 && (
        <CommandGroup heading="Tasks" forceMount>
          {taskResults.map((task) => (
            <TaskResultItem key={task.id} task={task} stepMap={stepMap} onSelect={onTaskSelect} />
          ))}
        </CommandGroup>
      )}
    </>
  );
}

type FileSearchContentProps = {
  files: string[];
  isSearching: boolean;
  search: string;
  onSelect: (path: string) => void;
};

function FileSearchContent({ files, isSearching, search, onSelect }: FileSearchContentProps) {
  if (isSearching && files.length === 0) {
    return (
      <div className="flex items-center justify-center py-6">
        <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (search.trim() && files.length === 0) return <CommandEmpty>No files found.</CommandEmpty>;
  if (!search.trim()) return <CommandEmpty>Type to search files...</CommandEmpty>;
  return (
    <CommandGroup heading="Files" forceMount>
      {files.map((filePath) => {
        const fileName = getFileName(filePath);
        const lastSlash = filePath.lastIndexOf("/");
        const dir = lastSlash > 0 ? filePath.slice(0, lastSlash) : "";
        return (
          <CommandItem
            key={filePath}
            value={`__file:${filePath}`}
            onSelect={() => onSelect(filePath)}
            forceMount
          >
            <FileIcon fileName={fileName} className="shrink-0" />
            <span className="font-medium truncate">{fileName}</span>
            {dir && <span className="text-muted-foreground text-xs truncate ml-1">{dir}</span>}
          </CommandItem>
        );
      })}
    </CommandGroup>
  );
}

function getInputPlaceholder(mode: CommandPanelMode, inputCommand: CommandItemType | null) {
  if (mode === "input") return inputCommand?.inputPlaceholder ?? "Enter value...";
  if (mode === "search-tasks") return "Search for tasks...";
  if (mode === MODE_SEARCH_FILES) return "Search for files...";
  return "Type a command...";
}

function getEnterLabel(mode: CommandPanelMode) {
  if (mode === "input") return "Confirm";
  if (mode === "search-tasks" || mode === MODE_SEARCH_FILES) return "Open";
  return "Select";
}

function getModeLabel(mode: CommandPanelMode, inputCommand: CommandItemType | null) {
  if (mode === "input") return inputCommand?.label;
  if (mode === "search-tasks") return "Tasks";
  if (mode === MODE_SEARCH_FILES) return "Files";
  return null;
}

function CommandPanelFooter({ mode }: { mode: CommandPanelMode }) {
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  return (
    <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
      {mode === MODE_COMMANDS && (
        <>
          <KbdGroup>
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span>Navigate</span>
          </KbdGroup>
          <KbdGroup>
            <Kbd>{formatShortcut(getShortcut("FILE_SEARCH", keyboardShortcuts))}</Kbd>
            <span>File Search</span>
          </KbdGroup>
        </>
      )}
      <KbdGroup>
        <Kbd>↵</Kbd>
        <span>{getEnterLabel(mode)}</span>
      </KbdGroup>
      {mode !== MODE_COMMANDS && (
        <KbdGroup>
          <Kbd>⌫</Kbd>
          <span>Back</span>
        </KbdGroup>
      )}
      <KbdGroup>
        <Kbd>esc</Kbd>
        <span>Close</span>
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
  setSelectedValue: (value: string) => void;
  search: string;
  setSearch: (value: string) => void;
  handleKeyDown: (e: React.KeyboardEvent) => void;
  goBack: () => void;
  fileResults: string[];
  isSearchingFiles: boolean;
  handleFileSelect: (filePath: string) => void;
  commands: CommandItemType[];
  grouped: Array<[string, CommandItemType[]]>;
  handleSelect: (cmd: CommandItemType) => void;
  isSearching: boolean;
  taskResults: Task[];
  stepMap: Map<string, { name: string; color: string }>;
  handleTaskSelect: (task: Task) => void;
};

export function CommandPanelView({
  open,
  setOpen,
  mode,
  inputCommand,
  selectedValue,
  setSelectedValue,
  search,
  setSearch,
  handleKeyDown,
  goBack,
  fileResults,
  isSearchingFiles,
  handleFileSelect,
  commands,
  grouped,
  handleSelect,
  isSearching,
  taskResults,
  stepMap,
  handleTaskSelect,
}: CommandPanelViewProps) {
  const modeLabel = getModeLabel(mode, inputCommand);
  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      overlayClassName="supports-backdrop-filter:backdrop-blur-none!"
    >
      <Command
        shouldFilter={mode === MODE_COMMANDS}
        loop
        value={selectedValue}
        onValueChange={setSelectedValue}
      >
        <div className="flex items-center border-b border-border [&>[data-slot=command-input-wrapper]]:flex-1">
          {mode !== MODE_COMMANDS && (
            <button
              onClick={goBack}
              className="shrink-0 pl-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <span>←</span>
              <span>{modeLabel}</span>
              <span className="text-muted-foreground/50">›</span>
            </button>
          )}
          <CommandInput
            placeholder={getInputPlaceholder(mode, inputCommand)}
            value={search}
            onValueChange={setSearch}
            onKeyDown={handleKeyDown}
          />
        </div>
        <CommandList>
          {mode === MODE_COMMANDS && (
            <CommandsListContent
              commands={commands}
              grouped={grouped}
              search={search}
              onSelect={handleSelect}
              taskResults={taskResults}
              isSearching={isSearching}
              stepMap={stepMap}
              onTaskSelect={handleTaskSelect}
            />
          )}
          {mode === MODE_SEARCH_FILES && (
            <FileSearchContent
              files={fileResults}
              isSearching={isSearchingFiles}
              search={search}
              onSelect={handleFileSelect}
            />
          )}
          {mode === "input" &&
            (!search.trim() ? (
              <CommandEmpty>{inputCommand?.inputPlaceholder ?? "Enter a value..."}</CommandEmpty>
            ) : (
              <CommandEmpty>Press Enter to confirm</CommandEmpty>
            ))}
        </CommandList>
        <CommandPanelFooter mode={mode} />
      </Command>
    </CommandDialog>
  );
}
