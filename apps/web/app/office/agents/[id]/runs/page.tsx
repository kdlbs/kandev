import { serverFetchOfficeJson } from "@/lib/api/server/office";
import type { AgentRunsListPage } from "@/lib/api/domains/office-extended-api";
import { RunsListView } from "./runs-list-view";

type Props = { params: Promise<{ id: string }> };

/**
 * Per-agent paginated runs list. Server Component fetches page 1 via
 * the SSR helper and hands the result to the client view; subsequent
 * pages are loaded on click via `listAgentRuns`.
 */
export default async function AgentRunsPage({ params }: Props) {
  const { id } = await params;
  const initial = await serverFetchOfficeJson<AgentRunsListPage>(`/agents/${id}/runs?limit=25`);
  return <RunsListView initial={initial} agentId={id} />;
}
