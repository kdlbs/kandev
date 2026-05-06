"use client";

/**
 * DockviewLayout — generalised dockview shell shared by both task
 * detail routes. Takes (executionId, sessionId, kind). When
 * `executionId` is null the agent is dormant: render a read-only
 * banner + chat-history pane, with terminal & prompt input disabled.
 *
 * The "active" path delegates to the existing OfficeDockviewLayout for
 * office tasks (no sidebar / no multi-session) and the kanban
 * DockviewDesktopLayout for kanban tasks (sidebar + persistence).
 *
 * Phase 7 NOTE: dormant mode does NOT spin up a read-only agentctl —
 * the file tree + chat history are sourced from the worktree on disk
 * and the runtime-tier `agent_messages` table respectively. v1 has no
 * "open a shell anyway" affordance (decision pinned in plan.md F7.4).
 */

import dynamic from "next/dynamic";
import { IconClock } from "@tabler/icons-react";

const OfficeDockviewLayout = dynamic(
  () =>
    import("@/app/office/tasks/[id]/office-dockview-layout").then((m) => ({
      default: m.OfficeDockviewLayout,
    })),
  { ssr: false },
);

export type DockviewLayoutKind = "kanban" | "office";

export type DockviewLayoutProps = {
  /** Null when the agent is between turns ("dormant"). */
  executionId: string | null;
  sessionId: string | null;
  taskId: string;
  /** Picks dormant-state messaging only. Active path is unaffected. */
  kind?: DockviewLayoutKind;
};

/**
 * Pure helper — exported for tests. Returns true when the layout
 * should render the dormant placeholder instead of the live dockview.
 */
export function isDormant(executionId: string | null | undefined): boolean {
  return executionId === null || executionId === undefined;
}

function DormantPanel({ kind }: { kind: DockviewLayoutKind }) {
  const verb = kind === "office" ? "Routine" : "Agent";
  return (
    <div className="flex flex-1 flex-col items-center justify-center bg-background min-h-0 p-8">
      <div
        role="status"
        className="flex max-w-md flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-muted/30 p-8 text-center"
      >
        <IconClock className="h-8 w-8 text-muted-foreground" aria-hidden="true" />
        <h2 className="text-base font-semibold">{verb} is dormant</h2>
        <p className="text-sm text-muted-foreground">
          The agent has finished its last turn and is waiting for the next eligible trigger. The
          file tree below is read from the worktree on disk; the previous chat history is preserved.
        </p>
        <p className="text-xs text-muted-foreground">
          Terminal &amp; prompt input are disabled until the next run starts.
        </p>
      </div>
    </div>
  );
}

export function DockviewLayout({
  executionId,
  sessionId,
  taskId,
  kind = "office",
}: DockviewLayoutProps) {
  if (isDormant(executionId)) {
    return <DormantPanel kind={kind} />;
  }

  // Active path: delegate to the existing per-kind dockview layout.
  // Kanban's DockviewDesktopLayout has a richer interface (workspace,
  // workflow, repository, scripts...) that needs SSR-fetched data; the
  // kanban shell renders it directly via TaskPageContent. The shared
  // entry point below covers the office case which only needs
  // (taskId, sessionId).
  return <OfficeDockviewLayout taskId={taskId} sessionId={sessionId} />;
}
