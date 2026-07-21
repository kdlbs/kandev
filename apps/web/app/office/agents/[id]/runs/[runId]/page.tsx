import { serverFetchOfficeJson } from "@/lib/api/server/office";
import type { AgentRunsListPage, RunDetail } from "@/lib/api/domains/office-extended-api";
import { RunDetailView } from "./run-detail-view";

type Props = { params: Promise<{ id: string; runId: string }> };

/**
 * Run detail route loader: parallel-fetches the run aggregate and the
 * recent-runs sidebar window so the page returns with both the main panel
 * and the sidebar populated. Errors bubble to the route error surface;
 * live-mode WS subscription is added by the view.
 */
export default async function AgentRunDetailPage({ params }: Props) {
  const { id, runId } = await params;
  const [initial, recent] = await Promise.all([
    serverFetchOfficeJson<RunDetail>(`/agents/${id}/runs/${runId}`),
    serverFetchOfficeJson<AgentRunsListPage>(`/agents/${id}/runs?limit=30`),
  ]);
  return <RunDetailView agentId={id} initial={initial} recent={recent} />;
}
