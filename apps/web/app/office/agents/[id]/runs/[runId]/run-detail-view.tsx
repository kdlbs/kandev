"use client";

import { useEffect, useMemo } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import type { AgentRunsListPage, RunDetail } from "@/lib/api/domains/office-extended-api";
import { qk } from "@/lib/query/keys";
import {
  officeAgentRunsInfiniteQueryOptions,
  officeRunDetailQueryOptions,
} from "@/lib/query/query-options/office";
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
 * Run detail client shell. The route loader delivers `initial` (the run
 * aggregate) and `recent` (the sidebar window) in one round-trip; this
 * component owns interactivity (collapsibles, action buttons, the embedded
 * conversation). While the run is `claimed`, `useRunLiveSync` subscribes to
 * `run.subscribe` over the WS and feeds appended events into the EventsLog
 * plus updates the status badge on terminal events, with no whole-snapshot
 * refetch.
 */
export function RunDetailView({ agentId, initial, recent }: Props) {
  const queryClient = useQueryClient();
  const detailQuery = useQuery(officeRunDetailQueryOptions(agentId, initial.id));
  const recentQuery = useInfiniteQuery(officeAgentRunsInfiniteQueryOptions(agentId, { limit: 30 }));

  useEffect(() => {
    queryClient.setQueryData(qk.office.runDetail(agentId, initial.id), initial);
    queryClient.setQueryData(qk.office.agentRuns(agentId, { limit: 30 }), {
      pages: [recent],
      pageParams: [undefined],
    });
  }, [agentId, initial, queryClient, recent]);

  const run = detailQuery.data ?? initial;
  const recentRuns = recentQuery.data?.pages[0]?.runs ?? recent.runs;
  const taskId = run.task_id ?? "";
  const sessionId = run.session.session_id ?? "";
  const { events, status } = useRunLiveSync(run.id, run.events, run.status);
  const liveRun = useMemo<RunDetail>(
    () => (status === run.status ? run : { ...run, status }),
    [run, status],
  );
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[280px_1fr] gap-4">
      <aside className="lg:sticky lg:top-4 lg:self-start">
        <RecentRunsSidebar runs={recentRuns} agentId={agentId} activeRunId={run.id} />
      </aside>
      <main className="space-y-4 min-w-0">
        <RunHeader run={liveRun} />
        <RoutePanel runId={run.id} />
        <SessionCollapsible session={run.session} />
        <InvocationPanel invocation={run.invocation} />
        <RuntimePanel runtime={run.runtime} />
        <PromptPanel run={run} />
        <TasksTouched runId={run.id} taskIds={run.tasks_touched} />
        <RunConversation taskId={taskId} sessionId={sessionId} />
        <EventsLog events={events} />
      </main>
    </div>
  );
}
