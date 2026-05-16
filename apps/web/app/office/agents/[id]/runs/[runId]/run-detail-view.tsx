"use client";

import { useMemo } from "react";
import type { AgentRunsListPage, RunDetail } from "@/lib/api/domains/office-extended-api";
import { RunHeader } from "../components/run-header";
import { RecentRunsSidebar } from "../components/recent-runs-sidebar";
import { SessionCollapsible } from "../components/session-collapsible";
import { InvocationPanel } from "../components/invocation-panel";
import { RuntimePanel } from "../components/runtime-panel";
import { PromptPanel } from "../components/prompt-panel";
import { EventsLog } from "../components/events-log";
import { RunConversation } from "../components/conversation";
import { TasksTouched } from "../components/tasks-touched";
import { RoutePanel } from "../../../../components/routing/route-panel";
import { useRunLiveSync } from "./use-run-live-sync";

type Props = {
  agentId: string;
  initial: RunDetail;
  recent: AgentRunsListPage;
};

/**
 * Run detail client shell. The Server Component delivers `initial`
 * (the run aggregate) and `recent` (the sidebar window) in one
 * round-trip; this component owns interactivity (collapsibles,
 * action buttons, the embedded conversation). While the run is
 * `claimed`, `useRunLiveSync` subscribes to `run.subscribe` over the
 * WS and feeds appended events into the EventsLog plus updates the
 * status badge on terminal events — no whole-snapshot refetch.
 */
export function RunDetailView({ agentId, initial, recent }: Props) {
  const taskId = initial.task_id ?? "";
  const sessionId = initial.session.session_id ?? "";
  const { events, status } = useRunLiveSync(initial.id, initial.events, initial.status);
  const liveRun = useMemo<RunDetail>(
    () => (status === initial.status ? initial : { ...initial, status }),
    [initial, status],
  );
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[280px_1fr] gap-4">
      <aside className="lg:sticky lg:top-4 lg:self-start">
        <RecentRunsSidebar runs={recent.runs} agentId={agentId} activeRunId={initial.id} />
      </aside>
      <main className="space-y-4 min-w-0">
        <RunHeader run={liveRun} />
        <RoutePanel runId={initial.id} />
        <SessionCollapsible session={initial.session} />
        <InvocationPanel invocation={initial.invocation} />
        <RuntimePanel runtime={initial.runtime} />
        <PromptPanel run={initial} />
        <TasksTouched runId={initial.id} taskIds={initial.tasks_touched} />
        <RunConversation taskId={taskId} sessionId={sessionId} />
        <EventsLog events={events} />
      </main>
    </div>
  );
}
