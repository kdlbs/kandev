"use client";

import {
  IconBriefcase,
  IconColumns3,
  IconList,
  IconListCheck,
  IconRobot,
  IconServer,
  IconFiles,
  IconLink,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import {
  EmptyListNote,
  IdChip,
  KandevBody,
  KandevRow,
  ListItemRow,
  SummaryDot,
  TaskStateBadge,
} from "./shared";
import { pickArray, pickNumber, pickString } from "./parse";
import type { KandevRenderer } from "./types";
import { useTranslation } from "react-i18next";

const UNNAMED_KEY = "task:unnamed";

// NamedListRow is the canonical row layout for entries with name + id +
// optional description (workspaces, workflows, executor profiles). The id
// sits next to the name as a subtle hint; the description occupies its own
// line in a quieter colour.
function NamedListRow({
  name,
  id,
  description,
}: {
  name: string | undefined;
  id?: string;
  description?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="text-xs space-y-0.5">
      <div className="flex items-baseline gap-2">
        <span>{name ?? t(UNNAMED_KEY)}</span>
        <IdChip id={id} />
      </div>
      {description && <div className="text-[11px] text-muted-foreground/70">{description}</div>}
    </div>
  );
}

// ---------- list_workspaces ----------

type WorkspaceItem = {
  id?: string;
  name?: string;
  description?: string;
};

export const ListWorkspacesRenderer: KandevRenderer = ({ result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<WorkspaceItem>(result, "workspaces") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  return (
    <KandevRow
      Icon={IconBriefcase}
      title={t("task:kandevListWorkspaces")}
      summary={t("task:workspaceCount", { count: total })}
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noWorkspacesFound" />
        ) : (
          items.map((w, i) => (
            <NamedListRow
              key={w.id ?? w.name ?? `workspace-${i}`}
              name={w.name}
              description={w.description}
              id={w.id}
            />
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_workflows ----------

type WorkflowItem = {
  id?: string;
  name?: string;
  description?: string;
  workspace_id?: string;
};

export const ListWorkflowsRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<WorkflowItem>(result, "workflows") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  const workspaceId = pickString(args, "workspace_id");
  return (
    <KandevRow
      Icon={IconColumns3}
      title={t("task:kandevListWorkflows")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {workspaceId && (
            <>
              <IdChip id={workspaceId} />
              <SummaryDot />
            </>
          )}
          {t("task:workflowCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noWorkflowsFound" />
        ) : (
          items.map((w, i) => (
            <NamedListRow
              key={w.id ?? w.name ?? `workflow-${i}`}
              name={w.name}
              description={w.description}
              id={w.id}
            />
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_workflow_steps ----------

type WorkflowStep = {
  id?: string;
  name?: string;
  position?: number;
  color?: string;
  is_start_step?: boolean;
  stage_type?: string;
  workflow_id?: string;
};

// Map the backend's Tailwind colour name (e.g. "bg-neutral-400") into a
// small dot next to the step name. Falls back to a neutral fill for unknown
// values so a renamed colour upstream doesn't break the row layout.
function StepColorDot({ color }: { color: string | undefined }) {
  const cls = color && color.startsWith("bg-") ? color : "bg-muted-foreground/40";
  return <span className={cn("inline-block w-2 h-2 rounded-full shrink-0", cls)} />;
}

export const ListWorkflowStepsRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<WorkflowStep>(result, "steps") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  const sorted = [...items].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  const workflowId = pickString(args, "workflow_id");
  return (
    <KandevRow
      Icon={IconList}
      title={t("task:kandevListWorkflowSteps")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {workflowId && (
            <>
              <IdChip id={workflowId} />
              <SummaryDot />
            </>
          )}
          {t("task:stepCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noStepsFound" />
        ) : (
          sorted.map((step, i) => (
            <div
              key={step.id ?? step.name ?? `step-${i}`}
              className="flex items-center gap-2 text-xs"
            >
              <StepColorDot color={step.color} />
              <span>{step.name ?? t(UNNAMED_KEY)}</span>
              {step.is_start_step && (
                <span className="text-[10px] text-muted-foreground/60">{t("task:start")}</span>
              )}
              {step.stage_type && step.stage_type !== "custom" && (
                <span className="text-[10px] text-muted-foreground/60">{step.stage_type}</span>
              )}
            </div>
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_tasks ----------

type TaskItem = {
  id?: string;
  title?: string;
  state?: string;
  workflow_step_id?: string;
  position?: number;
  description?: string;
};

export const ListTasksRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<TaskItem>(result, "tasks") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  const workflowId = pickString(args, "workflow_id");
  return (
    <KandevRow
      Icon={IconListCheck}
      title={t("task:kandevListTasks")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {workflowId && (
            <>
              <IdChip id={workflowId} />
              <SummaryDot />
            </>
          )}
          {t("task:taskCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noTasksFound" />
        ) : (
          items.map((item, i) => (
            <div
              key={item.id ?? item.title ?? `task-${i}`}
              className="flex items-baseline gap-2 text-xs"
            >
              <TaskStateBadge state={item.state} />
              <span>{item.title ?? t("task:untitled")}</span>
              <IdChip id={item.id} />
            </div>
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_related_tasks ----------

type RelatedTaskItem = {
  id?: string;
  title?: string;
  state?: string;
  workflow_step_id?: string;
};

// `key` is the wire field on the tool result; `labelKey` is our heading copy,
// carried as a catalog key because this table is built at module load.
const RELATED_GROUPS: Array<{ key: string; labelKey: string }> = [
  { key: "parents", labelKey: "task:relatedParents" },
  { key: "children", labelKey: "task:relatedChildren" },
  { key: "siblings", labelKey: "task:relatedSiblings" },
  { key: "blockers", labelKey: "task:relatedBlockers" },
  { key: "blocked_by", labelKey: "task:relatedBlockedBy" },
];

function RelatedGroup({ labelKey, items }: { labelKey: string; items: RelatedTaskItem[] }) {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return (
    <div className="space-y-1">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60">
        {t(labelKey)}
      </div>
      {items.map((item, i) => (
        <div
          key={item.id ?? item.title ?? `related-${i}`}
          className="flex items-baseline gap-2 text-xs pl-2"
        >
          <TaskStateBadge state={item.state} />
          <span>{item.title ?? t("task:untitled")}</span>
          <IdChip id={item.id} />
        </div>
      ))}
    </div>
  );
}

export const ListRelatedTasksRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const groups = RELATED_GROUPS.map((g) => ({
    ...g,
    items: pickArray<RelatedTaskItem>(result, g.key) ?? [],
  }));
  const total = groups.reduce((sum, g) => sum + g.items.length, 0);
  const taskId = pickString(args, "task_id");
  return (
    <KandevRow
      Icon={IconLink}
      title={t("task:kandevListRelatedTasks")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          {t("task:relatedTaskCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={total > 0}
    >
      <KandevBody>
        {total === 0 ? (
          <EmptyListNote messageKey="task:noRelatedTasksFound" />
        ) : (
          groups.map((g) => <RelatedGroup key={g.key} labelKey={g.labelKey} items={g.items} />)
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_agents ----------

type AgentProfile = { id?: string; name?: string; model?: string };
type AgentItem = {
  id?: string;
  name?: string;
  supports_mcp?: boolean;
  profiles?: AgentProfile[];
};

export const ListAgentsRenderer: KandevRenderer = ({ result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<AgentItem>(result, "agents") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  return (
    <KandevRow
      Icon={IconRobot}
      title={t("task:kandevListAgents")}
      summary={t("task:agentCount", { count: total })}
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noAgentsFound" />
        ) : (
          items.map((a, i) => (
            <ListItemRow key={a.id ?? a.name ?? `agent-${i}`}>
              <div className="font-medium">{a.name ?? a.id ?? t(UNNAMED_KEY)}</div>
              {a.profiles && a.profiles.length > 0 && (
                <div className="text-[11px] text-muted-foreground/70">
                  {a.profiles
                    .map((p) => (p.model ? `${p.name ?? p.id} (${p.model})` : (p.name ?? p.id)))
                    .filter(Boolean)
                    .join(", ")}
                </div>
              )}
            </ListItemRow>
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_executor_profiles ----------

type ExecutorProfileItem = {
  id?: string;
  name?: string;
  executor_id?: string;
  mcp_policy?: string;
};

export const ListExecutorProfilesRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<ExecutorProfileItem>(result, "profiles") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  const executorId = pickString(args, "executor_id");
  return (
    <KandevRow
      Icon={IconServer}
      title={t("task:kandevListExecutorProfiles")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {executorId && (
            <>
              <span className="text-muted-foreground/70">{executorId}</span>
              <SummaryDot />
            </>
          )}
          {t("task:profileCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noProfilesFound" />
        ) : (
          items.map((p, i) => (
            <div
              key={p.id ?? p.name ?? `profile-${i}`}
              className="flex items-baseline gap-2 text-xs"
            >
              <span>{p.name ?? t(UNNAMED_KEY)}</span>
              {p.mcp_policy && (
                <span className="text-[10px] text-muted-foreground/60">
                  {t("task:mcpPolicyLabel")} {p.mcp_policy}
                </span>
              )}
              <IdChip id={p.id} />
            </div>
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- list_task_documents ----------

type DocumentItem = {
  key?: string;
  title?: string;
  type?: string;
  author?: string;
  size?: number;
};

export const ListTaskDocumentsRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const items = pickArray<DocumentItem>(result, "documents") ?? [];
  const total = pickNumber(result, "total") ?? items.length;
  const taskId = pickString(args, "task_id") ?? pickString(result, "task_id");
  return (
    <KandevRow
      Icon={IconFiles}
      title={t("task:kandevListTaskDocuments")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          {t("task:documentCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={items.length > 0}
    >
      <KandevBody>
        {items.length === 0 ? (
          <EmptyListNote messageKey="task:noDocumentsFound" />
        ) : (
          items.map((d, i) => (
            <div key={d.key ?? d.title ?? `doc-${i}`} className="flex items-baseline gap-2 text-xs">
              <span>{d.title ?? d.key ?? t("task:untitled")}</span>
              {d.key && d.title && (
                <span className="font-mono text-[10px] text-muted-foreground/50">{d.key}</span>
              )}
              {d.type && <span className="text-[10px] text-muted-foreground/60">{d.type}</span>}
              {d.author && (
                <span className="text-[10px] text-muted-foreground/60">
                  {t("task:byAuthor", { author: d.author })}
                </span>
              )}
            </div>
          ))
        )}
      </KandevBody>
    </KandevRow>
  );
};
