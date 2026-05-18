"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import Link from "next/link";
import {
  IconCopy,
  IconPlayerPause,
  IconPlayerPlay,
  IconPlus,
  IconRestore,
  IconTrash,
  IconUpload,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import { useAppStore } from "@/components/state-provider";
import { selectLiveSessionForTask } from "@/lib/state/slices/session/selectors";
import { TaskProperties } from "./task-properties";
import { ChatActivityTabs } from "./chat-activity-tabs";
import { ExecutionIndicator } from "@/app/office/components/execution-indicator";
import { OfficeTopbarPortal } from "@/app/office/components/office-topbar-portal";
import { TaskDocuments } from "./task-documents";
import { TaskDetailContextPanel } from "../task-detail-context-panel";
import { useTaskContext } from "@/hooks/use-task-context";
import { StatusIcon } from "@/app/office/tasks/[id]/status-icon";
import { StageProgressBar } from "./stage-progress-bar";
import { SubtaskStepper } from "./subtask-stepper";
import { hasBlockerChain } from "./workflow-sort";
import { NewTaskDialog } from "@/app/office/components/new-task-dialog";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";
import { TopbarWorkingIndicator } from "./components/topbar-working-indicator";
import { TreeCancelDialog } from "@/components/task/TreeCancelDialog";
import {
  cancelTaskTree,
  pauseTaskTree,
  previewTaskTree,
  restoreTaskTree,
  resumeTaskTree,
  type TreeHold,
  type TreePreview,
} from "@/lib/api/domains/tree-api";
import type {
  Task,
  TaskComment,
  TaskActivityEntry,
  TaskSession,
  TimelineEvent,
} from "@/app/office/tasks/[id]/types";
import { toast } from "sonner";

type OfficeSimplePaneProps = {
  task: Task;
  comments: TaskComment[];
  timeline?: TimelineEvent[];
  activity: TaskActivityEntry[];
  sessions: TaskSession[];
  onToggleAdvanced?: () => void;
  onCommentsChanged?: () => void;
};

function TaskBreadcrumb({ task }: { task: Task }) {
  return (
    <Breadcrumb>
      <BreadcrumbList className="text-sm">
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link href="/office/tasks">Tasks</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        {task.parentIdentifier && (
          <>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href={`/office/tasks/${task.parentId}`}>{task.parentTitle}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
          </>
        )}
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbPage>{task.title}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}

function TaskHeaderRow({ task, activeHold }: { task: Task; activeHold: TreeHold | null }) {
  return (
    <div className="flex items-center gap-2">
      <StatusIcon status={task.status} />
      <span className="text-sm font-mono text-muted-foreground">{task.identifier}</span>
      {task.projectName && <Badge variant="outline">{task.projectName}</Badge>}
      {activeHold?.mode === "pause" && <Badge variant="outline">Paused</Badge>}
      {activeHold?.mode === "cancel" && <Badge variant="outline">Cancelled (tree)</Badge>}
      <div className="ml-auto">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={() => navigator.clipboard.writeText(task.identifier)}
            >
              <IconCopy className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Copy identifier</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

function ChildIssuesList({
  items,
  activeHold,
}: {
  items: Task["children"];
  activeHold: TreeHold | null;
}) {
  if (items.length === 0) return null;
  const holdLabel = activeHold?.mode === "pause" ? "Paused" : "Cancelled (tree)";
  return (
    <div className="mt-8" data-testid="child-issues-list">
      <h2 className="text-sm font-semibold mb-4">Sub-tasks</h2>
      <div className="border border-border rounded-lg divide-y divide-border">
        {items.map((child) => (
          <Link
            key={child.id}
            href={`/office/tasks/${child.id}`}
            className="flex items-center gap-2 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors"
          >
            <StatusIcon status={child.status} className="h-3.5 w-3.5 shrink-0" />
            <span className="text-xs text-muted-foreground font-mono shrink-0">
              {child.identifier}
            </span>
            <span className="flex-1 truncate">{child.title}</span>
            {activeHold && <Badge variant="outline">{holdLabel}</Badge>}
          </Link>
        ))}
      </div>
    </div>
  );
}

type TreeActionRunner = (action: () => Promise<unknown>, message: string) => Promise<void>;

function PauseResumeButton({
  taskId,
  activeHold,
  hasTree,
  busy,
  runAction,
}: {
  taskId: string;
  activeHold: TreeHold | null;
  hasTree: boolean;
  busy: boolean;
  runAction: TreeActionRunner;
}) {
  if (activeHold?.mode === "pause") {
    return (
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer"
        disabled={busy}
        onClick={() => runAction(() => resumeTaskTree(taskId), "Task tree resumed")}
      >
        <IconPlayerPlay className="h-3.5 w-3.5 mr-1" /> Resume tree
      </Button>
    );
  }
  if (!hasTree) return null;
  return (
    <Button
      variant="outline"
      size="sm"
      className="cursor-pointer"
      disabled={busy || activeHold?.mode === "cancel"}
      onClick={() => runAction(() => pauseTaskTree(taskId), "Task tree paused")}
    >
      <IconPlayerPause className="h-3.5 w-3.5 mr-1" /> Pause tree
    </Button>
  );
}

function CancelRestoreButton({
  taskId,
  activeHold,
  hasTree,
  busy,
  onCancel,
  runAction,
}: {
  taskId: string;
  activeHold: TreeHold | null;
  hasTree: boolean;
  busy: boolean;
  onCancel: () => void;
  runAction: TreeActionRunner;
}) {
  if (activeHold?.mode === "cancel") {
    return (
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer"
        disabled={busy}
        onClick={() => runAction(() => restoreTaskTree(taskId), "Task tree restored")}
      >
        <IconRestore className="h-3.5 w-3.5 mr-1" /> Restore tree
      </Button>
    );
  }
  if (!hasTree) return null;
  return (
    <Button
      variant="destructive"
      size="sm"
      className="cursor-pointer"
      disabled={busy}
      onClick={onCancel}
    >
      <IconTrash className="h-3.5 w-3.5 mr-1" /> Cancel tree
    </Button>
  );
}

function TreeControls({
  task,
  preview,
  activeHold,
  onChanged,
}: {
  task: Task;
  preview: TreePreview | null;
  activeHold: TreeHold | null;
  onChanged: () => Promise<void>;
}) {
  const [cancelOpen, setCancelOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const hasTree = (preview?.task_count ?? 1) > 1 || task.children.length > 0;

  const runAction = async (action: () => Promise<unknown>, message: string) => {
    setBusy(true);
    try {
      await action();
      toast.success(message);
      await onChanged();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Tree action failed");
    } finally {
      setBusy(false);
    }
  };

  if (!hasTree && !activeHold) return null;

  return (
    <div className="flex flex-wrap gap-2 mt-6">
      <PauseResumeButton
        taskId={task.id}
        activeHold={activeHold}
        hasTree={hasTree}
        busy={busy}
        runAction={runAction}
      />
      <CancelRestoreButton
        taskId={task.id}
        activeHold={activeHold}
        hasTree={hasTree}
        busy={busy}
        onCancel={() => setCancelOpen(true)}
        runAction={runAction}
      />
      {preview && hasTree && (
        <span className="inline-flex items-center text-xs text-muted-foreground">
          {preview.task_count} tasks affected
        </span>
      )}
      <TreeCancelDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        taskCount={preview?.task_count ?? task.children.length + 1}
        activeRunCount={preview?.active_run_count ?? 0}
        onConfirm={() => {
          setCancelOpen(false);
          void runAction(() => cancelTaskTree(task.id), "Task tree cancelled");
        }}
      />
    </div>
  );
}

function TaskActionRow({
  task,
  treePreview,
  activeHold,
  onTreeChanged,
  onNewSubIssue,
}: {
  task: Task;
  treePreview: TreePreview | null;
  activeHold: TreeHold | null;
  onTreeChanged: () => Promise<void>;
  onNewSubIssue: () => void;
}) {
  const attachInputRef = useRef<HTMLInputElement>(null);
  const handleAttachFiles = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0) return;
      const { createOrUpdateDocument } = await import("@/lib/api/domains/office-extended-api");
      for (const file of Array.from(files)) {
        try {
          const content = await file.text();
          const key = file.name.replace(/[^a-zA-Z0-9._-]/g, "_");
          await createOrUpdateDocument(task.id, key, {
            title: file.name,
            content: content.slice(0, 100_000),
          });
          toast.success(`Uploaded ${file.name}`);
        } catch {
          toast.error(`Failed to upload ${file.name}`);
        }
      }
      e.target.value = "";
    },
    [task.id],
  );

  return (
    <>
      <div className="flex gap-2 mt-6">
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={onNewSubIssue}>
          <IconPlus className="h-3.5 w-3.5 mr-1" /> New Sub-Task
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={() => attachInputRef.current?.click()}
        >
          <IconUpload className="h-3.5 w-3.5 mr-1" /> Upload attachment
        </Button>
        <input
          ref={attachInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={handleAttachFiles}
        />
      </div>
      <TreeControls
        task={task}
        preview={treePreview}
        activeHold={activeHold}
        onChanged={onTreeChanged}
      />
    </>
  );
}

function useTaskTreePreview(taskId: string) {
  const [treePreview, setTreePreview] = useState<TreePreview | null>(null);
  const [activeHold, setActiveHold] = useState<TreeHold | null>(null);

  const refreshTreePreview = useCallback(async () => {
    try {
      const preview = await previewTaskTree(taskId);
      setTreePreview(preview);
      setActiveHold(preview.active_hold ?? null);
    } catch {
      setTreePreview(null);
      setActiveHold(null);
    }
  }, [taskId]);

  useEffect(() => {
    let cancelled = false;
    previewTaskTree(taskId)
      .then((preview) => {
        if (cancelled) return;
        setTreePreview(preview);
        setActiveHold(preview.active_hold ?? null);
      })
      .catch(() => {
        if (cancelled) return;
        setTreePreview(null);
        setActiveHold(null);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId]);

  return { treePreview, activeHold, refreshTreePreview };
}

// useChatReadOnly returns true when the chat input should be hidden.
//
// Office tasks always accept comments — a user comment is the canonical
// way to wake the agent again (the reactivity pipeline fires
// task_comment runs). So even after the assignee's session has gone
// IDLE / COMPLETED, the input must stay so the user can ask a follow-up.
//
// Kanban / quick-chat keeps the legacy "finished tasks are read-only"
// behaviour: once the last session terminates the input goes away.
function useChatReadOnly(task: Task, sessions: TaskSession[]): boolean {
  const liveSession = useAppStore((s) => selectLiveSessionForTask(s, task.id));
  if (isOfficeTask(task)) return false;
  if (liveSession) return false;
  if (sessions.length === 0) return false;
  const lastSession = [...sessions].sort((a, b) =>
    (b.startedAt ?? "").localeCompare(a.startedAt ?? ""),
  )[0];
  if (!lastSession) return false;
  return (
    lastSession.state === "COMPLETED" ||
    lastSession.state === "FAILED" ||
    lastSession.state === "CANCELLED"
  );
}

// isOfficeTask mirrors the backend's IsOfficeTask: any task that has a
// project_id, or was created via the agent / routine origin paths.
function isOfficeTask(task: Task): boolean {
  if (task.projectId) return true;
  return false;
}

export function OfficeSimplePane({
  task,
  comments,
  timeline,
  activity,
  sessions,
  onToggleAdvanced,
  onCommentsChanged,
}: OfficeSimplePaneProps) {
  const [subIssueOpen, setSubIssueOpen] = useState(false);
  const [scrollParent, setScrollParent] = useState<HTMLDivElement | null>(null);
  const { treePreview, activeHold, refreshTreePreview } = useTaskTreePreview(task.id);
  const isReadOnly = useChatReadOnly(task, sessions);

  return (
    <ActiveSessionRefProvider>
      <div className="flex h-full">
        <OfficeTopbarPortal>
          <TaskBreadcrumb task={task} />
          <TopbarWorkingIndicator taskId={task.id} />
          <span className="flex-1" />
          <ExecutionIndicator status={task.status} />
          {onToggleAdvanced && (
            <div className="flex items-center gap-2">
              <Label
                htmlFor="advanced-toggle"
                className="text-xs text-muted-foreground cursor-pointer"
              >
                Advanced
              </Label>
              <Switch
                id="advanced-toggle"
                checked={false}
                onCheckedChange={() => onToggleAdvanced()}
              />
            </div>
          )}
          <Link
            href={`/t/${task.id}`}
            className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer whitespace-nowrap"
            data-testid="task-cross-link"
          >
            Open in advanced view
          </Link>
        </OfficeTopbarPortal>
        <div ref={setScrollParent} className="flex-1 min-w-0 overflow-y-auto p-6">
          <TaskHeaderRow task={task} activeHold={activeHold} />
          {task.executionPolicy && (
            <StageProgressBar
              executionPolicy={task.executionPolicy}
              executionState={task.executionState}
            />
          )}
          <h1 className="text-xl font-semibold mt-4">{task.title}</h1>
          {task.description && (
            <div className="prose prose-sm mt-4 max-w-none text-sm whitespace-pre-wrap">
              {task.description}
            </div>
          )}
          <TaskActionRow
            task={task}
            treePreview={treePreview}
            activeHold={activeHold}
            onTreeChanged={refreshTreePreview}
            onNewSubIssue={() => setSubIssueOpen(true)}
          />
          <TaskDocuments taskId={task.id} />
          <TaskContextSection taskId={task.id} revisionKey={task.updatedAt} />
          <ChatActivityTabs
            task={task}
            comments={comments}
            timeline={timeline}
            activity={activity}
            sessions={sessions}
            scrollParent={scrollParent}
            readOnly={isReadOnly}
            onCommentsChanged={onCommentsChanged}
          />
          {hasBlockerChain(task.children) ? (
            <SubtaskStepper items={task.children} activeHold={activeHold} />
          ) : (
            <ChildIssuesList items={task.children} activeHold={activeHold} />
          )}
        </div>
        <div className="w-80 border-l border-border shrink-0 overflow-y-auto p-4">
          <TaskProperties task={task} />
        </div>
        <NewTaskDialog
          open={subIssueOpen}
          onOpenChange={setSubIssueOpen}
          parentTaskId={task.id}
          defaultProjectId={task.projectId}
          defaultAssigneeId={task.assigneeAgentProfileId}
        />
      </div>
    </ActiveSessionRefProvider>
  );
}

/**
 * Office task-handoffs phase 8.2 — task detail context section.
 *
 * Mounts the TaskDetailContextPanel under TaskDocuments, fetching the
 * context envelope on mount and re-fetching whenever the task ID
 * changes. The panel renders nothing when the backend returns null
 * (no HandoffService configured) so the prior layout is preserved.
 */
function TaskContextSection({ taskId, revisionKey }: { taskId: string; revisionKey: string }) {
  const ctx = useTaskContext(taskId, revisionKey);
  if (!ctx) return null;
  return (
    <div className="mt-4">
      <TaskDetailContextPanel context={ctx} />
    </div>
  );
}
