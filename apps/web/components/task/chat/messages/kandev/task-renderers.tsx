"use client";

import { IconArrowsExchange, IconMessage2, IconPencil, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { IdChip, KandevBody, KandevRow, KeyValueRow, SummaryDot, TaskStateBadge } from "./shared";
import { pickArray, pickString } from "./parse";
import type { KandevRenderer } from "./types";

type Repository = {
  repository_id?: string;
  local_path?: string;
  github_url?: string;
  base_branch?: string;
};

function RepoChips({ repos }: { repos: Repository[] }) {
  if (repos.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1">
      {repos.map((r, i) => (
        <Badge key={r.repository_id ?? r.local_path ?? i} variant="outline" className="text-[10px]">
          {r.local_path ?? r.github_url ?? r.repository_id}
          {r.base_branch ? ` @ ${r.base_branch}` : ""}
        </Badge>
      ))}
    </div>
  );
}

// TaskDTOBody renders the common task fields that come back from create /
// update / move endpoints. Keeps the per-tool renderers focused on header /
// summary differences only.
function TaskDTOBody({ task }: { task: Record<string, unknown> | undefined }) {
  if (!task) return null;
  const title = pickString(task, "title");
  const description = pickString(task, "description");
  const state = pickString(task, "state");
  const id = pickString(task, "id");
  const workflowId = pickString(task, "workflow_id");
  const stepId = pickString(task, "workflow_step_id");
  const repos = pickArray<Repository>(task, "repositories") ?? [];
  return (
    <>
      <div className="flex items-center gap-2 flex-wrap">
        <TaskStateBadge state={state} />
        {title && <span className="text-sm font-medium">{title}</span>}
        <IdChip id={id} />
      </div>
      {description && (
        <div className="text-xs text-muted-foreground whitespace-pre-wrap line-clamp-4">
          {description}
        </div>
      )}
      <div className="flex flex-wrap gap-3 text-[11px] text-muted-foreground/70">
        {workflowId && <IdChip id={workflowId} />}
        {stepId && <IdChip id={stepId} />}
      </div>
      <RepoChips repos={repos} />
    </>
  );
}

// ---------- create_task ----------

export const CreateTaskRenderer: KandevRenderer = ({ args, result, status }) => {
  const title = pickString(args, "title");
  const task =
    result && typeof result === "object" ? (result as Record<string, unknown>) : undefined;
  const createdId = pickString(task, "id");
  return (
    <KandevRow
      Icon={IconPlus}
      title="Kandev: Create Task"
      summary={
        <span className="inline-flex items-center gap-1.5">
          {title && <span className="truncate max-w-[40ch]">&ldquo;{title}&rdquo;</span>}
          {createdId && (
            <>
              <SummaryDot />
              <IdChip id={createdId} />
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!task}
    >
      <KandevBody>
        <TaskDTOBody task={task} />
      </KandevBody>
    </KandevRow>
  );
};

// ---------- update_task ----------

export const UpdateTaskRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const newTitle = pickString(args, "title");
  const newState = pickString(args, "state");
  const newDescription = pickString(args, "description");
  const task =
    result && typeof result === "object" ? (result as Record<string, unknown>) : undefined;

  // Build a compact list of which fields were touched on this call. Showing
  // just the field names (or values for state) keeps the header informative
  // even when the title is long.
  const changes: string[] = [];
  if (newTitle !== undefined) changes.push("title");
  if (newDescription !== undefined) changes.push("description");
  if (newState !== undefined) changes.push(`state=${newState}`);

  return (
    <KandevRow
      Icon={IconPencil}
      title="Kandev: Update Task"
      summary={
        <span className="inline-flex items-center gap-1.5">
          <IdChip id={taskId} />
          {changes.length > 0 && (
            <>
              <SummaryDot />
              <span>{changes.join(", ")}</span>
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!task || changes.length > 0}
    >
      <KandevBody>
        {newTitle !== undefined && <KeyValueRow label="title">{newTitle}</KeyValueRow>}
        {newState !== undefined && (
          <KeyValueRow label="state">
            <TaskStateBadge state={newState} />
          </KeyValueRow>
        )}
        {newDescription !== undefined && (
          <KeyValueRow label="description">
            <span className="whitespace-pre-wrap">{newDescription}</span>
          </KeyValueRow>
        )}
        {task && <TaskDTOBody task={task} />}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- move_task ----------

export const MoveTaskRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const stepId = pickString(args, "workflow_step_id");
  const workflowId = pickString(args, "workflow_id");
  const prompt = pickString(args, "prompt");
  const task =
    result && typeof result === "object" ? (result as Record<string, unknown>) : undefined;
  return (
    <KandevRow
      Icon={IconArrowsExchange}
      title="Kandev: Move Task"
      summary={
        <span className="inline-flex items-center gap-1.5">
          <IdChip id={taskId} />
          {taskId && stepId && <SummaryDot />}
          <IdChip id={stepId} />
        </span>
      }
      status={status}
      hasExpandableContent={!!task || !!prompt || !!workflowId || !!stepId}
    >
      <KandevBody>
        {workflowId && (
          <KeyValueRow label="workflow" mono>
            {workflowId}
          </KeyValueRow>
        )}
        {stepId && (
          <KeyValueRow label="step" mono>
            {stepId}
          </KeyValueRow>
        )}
        {prompt && (
          <KeyValueRow label="prompt">
            <span className="whitespace-pre-wrap">{prompt}</span>
          </KeyValueRow>
        )}
        {task && <TaskDTOBody task={task} />}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- message_task ----------

export const MessageTaskRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const prompt = pickString(args, "prompt");
  // Truncate the prompt to a single short fragment for the header so a
  // multi-line message still renders in a single chat row.
  const promptShort = prompt ? prompt.replace(/\s+/g, " ").trim() : undefined;
  // message_task can return either {success: true} or a TaskSessionDTO. Treat
  // anything object-shaped with an `id` as the session DTO.
  const sessionId = pickString(result, "id");
  const session = sessionId ? (result as Record<string, unknown>) : undefined;
  return (
    <KandevRow
      Icon={IconMessage2}
      title="Kandev: Message Task"
      summary={
        <span className="inline-flex items-center gap-1.5 min-w-0">
          <IdChip id={taskId} />
          {promptShort && (
            <>
              <SummaryDot />
              <span className="truncate max-w-[40ch]">&ldquo;{promptShort}&rdquo;</span>
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!prompt}
    >
      <KandevBody>
        {prompt && (
          <KeyValueRow label="prompt">
            <span className="whitespace-pre-wrap">{prompt}</span>
          </KeyValueRow>
        )}
        {session && (
          <KeyValueRow label="session">
            <IdChip id={sessionId} />
          </KeyValueRow>
        )}
      </KandevBody>
    </KandevRow>
  );
};
