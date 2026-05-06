"use client";

import { Badge } from "@kandev/ui/badge";
import { IconAlertTriangle, IconFileText, IconUsersGroup } from "@tabler/icons-react";
import type { TaskContextDTO, TaskRefDTO } from "@/lib/api/domains/office-task-context-api";

type Props = {
  /**
   * The task context envelope from GET /api/v1/tasks/:id/context.
   * Pass null when the backend's HandoffService is not configured or the
   * fetch failed; the panel renders nothing in that case so the page
   * falls back to its pre-handoffs layout.
   */
  context: TaskContextDTO | null;
};

/**
 * Office task-handoffs phase 8.2 — task detail panel.
 *
 * Surfaces the cross-task relations + document references + workspace
 * group state introduced by the handoffs feature. Document bodies are
 * never rendered here; the panel only shows references that the user
 * can click through to fetch full content.
 */
export function TaskDetailContextPanel({ context }: Props) {
  if (!context) return null;
  const showRelations =
    !!context.parent ||
    context.children.length > 0 ||
    context.siblings.length > 0 ||
    context.blockers.length > 0 ||
    context.blocked_by.length > 0;
  const showDocs = context.available_documents.length > 0;
  const showWorkspace = !!context.workspace_group;
  if (!showRelations && !showDocs && !showWorkspace) return null;

  return (
    <section
      data-testid="task-detail-context-panel"
      className="space-y-4 rounded-md border border-border bg-card p-4 text-sm"
    >
      <WorkspaceStatusBanner context={context} />
      {showWorkspace && context.workspace_group && (
        <WorkspaceSection group={context.workspace_group} mode={context.workspace_mode} />
      )}
      {showRelations && <RelationsSection context={context} />}
      {showDocs && <DocumentsSection docs={context.available_documents} />}
    </section>
  );
}

function WorkspaceStatusBanner({ context }: { context: TaskContextDTO }) {
  if (context.workspace_status !== "requires_configuration") return null;
  return (
    <div
      role="alert"
      data-testid="task-detail-context-panel-requires-config"
      className="flex items-start gap-2 rounded-md border border-amber-300/60 bg-amber-50 p-3 text-amber-900 dark:border-amber-700/60 dark:bg-amber-950/40 dark:text-amber-200"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
      <div className="space-y-1">
        <div className="text-xs font-medium">Workspace requires configuration</div>
        <div className="text-[11px]">
          Restoring this task&apos;s materialized workspace failed. Reconfigure the workspace before
          launching.
        </div>
      </div>
    </div>
  );
}

function WorkspaceSection({
  group,
  mode,
}: {
  group: NonNullable<TaskContextDTO["workspace_group"]>;
  mode?: TaskContextDTO["workspace_mode"];
}) {
  const memberCount = group.members.length;
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <IconUsersGroup className="h-4 w-4 text-muted-foreground" />
        <Badge variant="outline" className="font-normal">
          Shared workspace · {memberCount} {memberCount === 1 ? "member" : "members"}
        </Badge>
        {mode && (
          <span className="text-[11px] text-muted-foreground" data-testid="workspace-mode-label">
            mode: {mode}
          </span>
        )}
      </div>
      {group.materialized_path && (
        <div className="font-mono text-[11px] text-muted-foreground">{group.materialized_path}</div>
      )}
      {memberCount > 0 && (
        <div className="flex flex-wrap gap-1">
          {group.members.map((m) => (
            <Badge key={m.id} variant="secondary" className="font-normal">
              {refLabel(m)}
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}

function RelationsSection({ context }: { context: TaskContextDTO }) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-muted-foreground">Related tasks</div>
      <div className="space-y-1 text-xs">
        {context.parent && <Relation label="Parent" task={context.parent} />}
        {context.siblings.map((t) => (
          <Relation key={t.id} label="Sibling" task={t} />
        ))}
        {context.blockers.map((t) => (
          <Relation key={t.id} label="Blocked by" task={t} variant="warning" />
        ))}
        {context.blocked_by.map((t) => (
          <Relation key={t.id} label="Blocks" task={t} />
        ))}
      </div>
    </div>
  );
}

function Relation({
  label,
  task,
  variant,
}: {
  label: string;
  task: TaskRefDTO;
  variant?: "warning";
}) {
  return (
    <div className="flex items-center gap-2">
      <Badge variant={variant === "warning" ? "destructive" : "outline"} className="font-normal">
        {label}
      </Badge>
      <span data-testid={`relation-${task.id}`} className="text-foreground">
        {refLabel(task)}
      </span>
    </div>
  );
}

function DocumentsSection({ docs }: { docs: TaskContextDTO["available_documents"] }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <IconFileText className="h-4 w-4" /> Documents available
      </div>
      <ul className="space-y-1 text-xs">
        {docs.map((d) => (
          <li
            key={`${d.task.id}:${d.key}`}
            data-testid={`document-ref-${d.task.id}-${d.key}`}
            className="flex items-baseline gap-2"
          >
            <Badge variant="outline" className="font-normal">
              {refLabel(d.task)}
            </Badge>
            <span className="font-mono text-foreground">{d.key}</span>
            {d.title && <span className="text-muted-foreground">— {d.title}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function refLabel(t: TaskRefDTO): string {
  return t.identifier ? `${t.identifier} ${t.title}` : t.title;
}
