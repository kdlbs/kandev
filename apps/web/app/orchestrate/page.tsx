import { getDashboard } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "./lib/get-active-workspace";
import { OrchestratePageClient } from "./page-client";
import type { DashboardData } from "@/lib/state/slices/orchestrate/types";

export default async function OrchestratePage() {
  const workspaceId = await getActiveWorkspaceId();

  let dashboard: DashboardData | null = null;
  if (workspaceId) {
    dashboard = await getDashboard(workspaceId, { cache: "no-store" }).catch(() => null);
  }

  return <OrchestratePageClient initialDashboard={dashboard} />;
}
