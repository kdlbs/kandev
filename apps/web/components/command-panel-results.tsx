"use client";

import { useTranslation } from "react-i18next";
import { formatTimeDistance, useDateLocale } from "@/lib/i18n/date-locale";
import { IconArchive, IconArrowRight, IconLoader2 } from "@tabler/icons-react";
import { CommandEmpty, CommandGroup, CommandItem, CommandShortcut } from "@kandev/ui/command";
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
import { useRepoDisplayName } from "@/hooks/domains/session/use-repo-display-name";
import { groupByRepositoryName, isSingleRepoGroup } from "@/lib/group-by-repo";
import { getTaskStateIconLabelKey, TaskStateIcon } from "@/components/task/task-state-icon";
import {
  resolveTaskResultActivity,
  type CommandPanelLiveTask,
} from "@/lib/commands/task-result-activity";

export const MODE_COMMANDS: CommandPanelMode = "commands";
export const MODE_SEARCH_TASKS: CommandPanelMode = "search-tasks";
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

export type StepMap = Map<string, { name: string; color: string }>;

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
  stepMap: StepMap;
  repoMap: Map<string, string>;
  liveTasksById: Map<string, CommandPanelLiveTask>;
  lastStepIdByWorkflowId: ReadonlyMap<string, string>;
  onSelect: (task: Task) => void;
};

function TaskResultItem({
  task,
  stepMap,
  repoMap,
  liveTasksById,
  lastStepIdByWorkflowId,
  onSelect,
}: TaskResultItemProps) {
  const { t } = useTranslation();
  const locale = useDateLocale();
  const isArchived = task.archived_at != null;
  const archivedLabel = t("tasks:archived");
  const activity = resolveTaskResultActivity(
    task,
    liveTasksById.get(task.id),
    lastStepIdByWorkflowId,
  );
  const step = stepMap.get(activity.workflowStepId);
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
      forceMount
    >
      <div className="flex items-center gap-2 min-w-0 w-full">
        {isArchived ? (
          <IconArchive
            role="img"
            aria-label={archivedLabel}
            data-testid="task-state-archived"
            className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground"
          />
        ) : (
          <TaskStateIcon {...activity} accessibleLabel={t(getTaskStateIconLabelKey(activity))} />
        )}
        {/* The title identifies the row, so it takes the free space and the
            non-shrinking badge and metadata cannot squeeze it away. Metadata is
            secondary and steps aside entirely on a phone. */}
        <span
          className={`min-w-0 flex-1 truncate font-medium${
            isArchived ? " text-muted-foreground" : ""
          }`}
        >
          {task.title}
        </span>
        {isArchived ? (
          <Badge
            variant="outline"
            className="text-[0.6rem] shrink-0 border-muted-foreground/50 text-muted-foreground"
          >
            {archivedLabel}
          </Badge>
        ) : (
          step && (
            <Badge
              variant="secondary"
              className="text-[0.6rem] shrink-0"
              style={stepHex ? { backgroundColor: stepHex + "22", color: stepHex } : undefined}
            >
              {step.name}
            </Badge>
          )
        )}
        {details.length > 0 && (
          <span className="hidden shrink-0 truncate text-[0.6rem] text-muted-foreground sm:inline">
            {details.join(" · ")}
          </span>
        )}
      </div>
    </CommandItem>
  );
}

type TaskResultGroupProps = {
  tasks: Task[];
  search: string;
  stepMap: StepMap;
  repoMap: Map<string, string>;
  liveTasksById: Map<string, CommandPanelLiveTask>;
  lastStepIdByWorkflowId: ReadonlyMap<string, string>;
  onSelect: (task: Task) => void;
  testId?: string;
};

function TaskResultGroup({
  tasks,
  search,
  stepMap,
  repoMap,
  liveTasksById,
  lastStepIdByWorkflowId,
  onSelect,
  testId,
}: TaskResultGroupProps) {
  const { t } = useTranslation();
  if (tasks.length === 0) return null;
  return (
    <CommandGroup
      heading={search.trim() ? t("common:tasks") : t("common:activeTasks")}
      forceMount
      data-testid={testId}
    >
      {tasks.map((task) => (
        <TaskResultItem
          key={task.id}
          task={task}
          stepMap={stepMap}
          repoMap={repoMap}
          liveTasksById={liveTasksById}
          lastStepIdByWorkflowId={lastStepIdByWorkflowId}
          onSelect={onSelect}
        />
      ))}
    </CommandGroup>
  );
}

function TaskResultsLoader() {
  const { t } = useTranslation();
  return (
    <CommandGroup heading={t("common:activeTasks")} forceMount>
      <div className="flex items-center justify-center py-3">
        <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
      </div>
    </CommandGroup>
  );
}

type CommandsListContentProps = {
  commands: CommandItemType[];
  grouped: [string, CommandItemType[]][];
  search: string;
  onSelect: (cmd: CommandItemType) => void;
  taskResults: Task[];
  isSearching: boolean;
  stepMap: StepMap;
  repoMap: Map<string, string>;
  liveTasksById: Map<string, CommandPanelLiveTask>;
  lastStepIdByWorkflowId: ReadonlyMap<string, string>;
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

function IdleCommandGroups({
  grouped,
  onSelect,
}: Pick<CommandsListContentProps, "grouped" | "onSelect">) {
  return (
    <>
      {grouped.map(([group, items]) => {
        const visibleItems = items.filter((cmd) => !cmd.searchOnly);
        if (visibleItems.length === 0) return null;
        return (
          <CommandGroup key={group} heading={group}>
            {visibleItems.map((cmd) => (
              <CommandItemRow key={cmd.id} cmd={cmd} onSelect={onSelect} />
            ))}
          </CommandGroup>
        );
      })}
    </>
  );
}

/**
 * The commands scope mixes commands with task rows. Idle, the Active Tasks
 * group leads: reopening the palette is usually a jump back to work, and there
 * is no query to rank commands against. Once the user types, the ordering
 * flips — a typed query is a command query first, so matching commands lead and
 * the fuzzy task matches trail as a preview. The Tasks scope owns the full
 * task search.
 */
export function CommandsListContent({
  commands,
  grouped,
  search,
  onSelect,
  taskResults,
  isSearching,
  stepMap,
  repoMap,
  liveTasksById,
  lastStepIdByWorkflowId,
  onTaskSelect,
}: CommandsListContentProps) {
  const { t } = useTranslation();
  const hasQuery = Boolean(search.trim());
  const hasInlineResults = taskResults.length > 0 || isSearching;
  const taskRows = (
    <>
      {isSearching && taskResults.length === 0 && <TaskResultsLoader />}
      <TaskResultGroup
        tasks={taskResults}
        search={search}
        stepMap={stepMap}
        repoMap={repoMap}
        liveTasksById={liveTasksById}
        lastStepIdByWorkflowId={lastStepIdByWorkflowId}
        onSelect={onTaskSelect}
        testId="command-panel-task-preview"
      />
    </>
  );
  const commandRows = hasQuery ? (
    <SearchCommandGroups commands={commands} search={search} onSelect={onSelect} />
  ) : (
    <IdleCommandGroups grouped={grouped} onSelect={onSelect} />
  );
  return (
    <>
      {!hasInlineResults && <CommandEmpty>{t("common:noCommandsFound")}</CommandEmpty>}
      {hasQuery ? commandRows : taskRows}
      {hasQuery ? taskRows : commandRows}
    </>
  );
}

type TaskSearchContentProps = {
  tasks: Task[];
  isSearching: boolean;
  search: string;
  stepMap: StepMap;
  repoMap: Map<string, string>;
  liveTasksById: Map<string, CommandPanelLiveTask>;
  lastStepIdByWorkflowId: ReadonlyMap<string, string>;
  onSelect: (task: Task) => void;
};

export function TaskSearchContent({
  tasks,
  isSearching,
  search,
  stepMap,
  repoMap,
  liveTasksById,
  lastStepIdByWorkflowId,
  onSelect,
}: TaskSearchContentProps) {
  const { t } = useTranslation();
  if (isSearching && tasks.length === 0) {
    return (
      <div className="flex items-center justify-center py-6">
        <IconLoader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (tasks.length === 0) return <CommandEmpty>{t("common:noTasksFound")}</CommandEmpty>;
  return (
    <TaskResultGroup
      tasks={tasks}
      search={search}
      stepMap={stepMap}
      repoMap={repoMap}
      liveTasksById={liveTasksById}
      lastStepIdByWorkflowId={lastStepIdByWorkflowId}
      onSelect={onSelect}
      testId="command-panel-task-results"
    />
  );
}

type FileSearchContentProps = {
  files: FileSearchResult[];
  isSearching: boolean;
  search: string;
  sessionId: string | null;
  onSelect: (path: string) => void;
};

export function FileSearchContent({
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
