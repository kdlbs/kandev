import { serverFetchOfficeJson } from "@/lib/api/server/office";
import type { AgentSummaryResponse } from "@/lib/api/domains/office-extended-api";
import { DashboardView } from "./dashboard-view";

type Props = { params: Promise<{ id: string }> };

const DEFAULT_DAYS = 14;

/**
 * Server Component for `/office/agents/[id]/dashboard`. Fetches the
 * summary on the server so the chart values appear in the initial
 * HTML (no JS required for first paint), then hands the snapshot to
 * the Client Component for interactivity + WS-driven refetches.
 *
 * Errors bubble up to the Next.js error boundary; we don't catch
 * them here. A 5xx from the backend produces an error page rather
 * than a half-empty dashboard, which matches the live-data nature
 * of the office surface.
 */
export default async function AgentDashboardPage({ params }: Props) {
  const { id } = await params;
  const summary = await serverFetchOfficeJson<AgentSummaryResponse>(
    `/agents/${id}/summary?days=${DEFAULT_DAYS}`,
  );
  return <DashboardView agentId={id} initial={summary} days={DEFAULT_DAYS} />;
}
