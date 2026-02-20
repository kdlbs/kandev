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
import type { Task } from "@/lib/types/http";
import { FileIcon } from "@/components/ui/file-icon";

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);
const MODE_SEARCH_TASKS: CommandPanelMode = "search-tasks";

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
      value={task.id}
      onSelect={() => onSelect(task)}
      className={isArchived ? "opacity-60" : ""}
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
  hasFileResults: boolean;
};

function CommandsListContent({
  commands,
  grouped,
  search,
  onSelect,
  hasFileResults,
}: CommandsListContentProps) {
  return (
    <>
      {!hasFileResults && <CommandEmpty>No commands found.</CommandEmpty>}
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
    </>
  );
}

type SearchTasksContentProps = {
  isSearching: boolean;
  search: string;
  taskResults: Task[];
  stepMap: Map<string, { name: string; color: string }>;
  onTaskSelect: (task: Task) => void;
};

function SearchTasksContent({
  isSearching,
  search,
  taskResults,
  stepMap,
  onTaskSelect,
}: SearchTasksContentProps) {
  if (isSearching)
    return (
      <div className="flex items-center justify-center py-6">
        <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  if (search.trim() && taskResults.length === 0)
    return <CommandEmpty>No tasks found.</CommandEmpty>;
  if (!search.trim()) return <CommandEmpty>Type to search tasks...</CommandEmpty>;
  return (
    <CommandGroup heading="Results">
      {taskResults.map((task) => (
        <TaskResultItem key={task.id} task={task} stepMap={stepMap} onSelect={onTaskSelect} />
      ))}
    </CommandGroup>
  );
}

type FileSearchResultsProps = {
  files: string[];
  isSearching: boolean;
  onSelect: (path: string) => void;
};

function FileSearchResults({ files, isSearching, onSelect }: FileSearchResultsProps) {
  if (isSearching && files.length === 0) {
    return (
      <CommandGroup heading="Files" forceMount>
        <div className="flex items-center justify-center py-3">
          <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
        </div>
      </CommandGroup>
    );
  }
  if (files.length === 0) return null;
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
  if (mode === MODE_SEARCH_TASKS) return "Search for tasks...";
  return "Type a command...";
}

function getEnterLabel(mode: CommandPanelMode) {
  if (mode === "input") return "Confirm";
  if (mode === MODE_SEARCH_TASKS) return "Open";
  return "Select";
}

function CommandPanelFooter({ mode }: { mode: CommandPanelMode }) {
  return (
    <div className="border-t border-border px-3 py-1.5 flex items-center gap-3 text-[0.6rem] text-muted-foreground">
      {mode === "commands" && (
        <KbdGroup>
          <Kbd>↑</Kbd>
          <Kbd>↓</Kbd>
          <span>Navigate</span>
        </KbdGroup>
      )}
      <KbdGroup>
        <Kbd>↵</Kbd>
        <span>{getEnterLabel(mode)}</span>
      </KbdGroup>
      {mode !== "commands" && (
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
  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      overlayClassName="supports-backdrop-filter:backdrop-blur-none!"
    >
      <Command
        shouldFilter={mode === "commands"}
        loop
        value={selectedValue}
        onValueChange={setSelectedValue}
      >
        <div className="flex items-center border-b border-border [&>[data-slot=command-input-wrapper]]:flex-1">
          {mode !== "commands" && (
            <button
              onClick={goBack}
              className="shrink-0 pl-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <span>←</span>
              <span>{mode === "input" ? inputCommand?.label : "Tasks"}</span>
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
          {mode === "commands" && (
            <>
              {search.trim() && (
                <FileSearchResults
                  files={fileResults}
                  isSearching={isSearchingFiles}
                  onSelect={handleFileSelect}
                />
              )}
              <CommandsListContent
                commands={commands}
                grouped={grouped}
                search={search}
                onSelect={handleSelect}
                hasFileResults={fileResults.length > 0 || isSearchingFiles}
              />
            </>
          )}
          {mode === MODE_SEARCH_TASKS && (
            <SearchTasksContent
              isSearching={isSearching}
              search={search}
              taskResults={taskResults}
              stepMap={stepMap}
              onTaskSelect={handleTaskSelect}
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
