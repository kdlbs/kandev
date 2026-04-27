import { redirect } from "next/navigation";
import { getDashboard, getOnboardingState } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "./lib/get-active-workspace";
import { OrchestratePageClient } from "./page-client";
import type { DashboardData } from "@/lib/state/slices/orchestrate/types";

export default async function OrchestratePage() {
  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: true,
  }));
  if (!onboarding.completed) {
    redirect("/orchestrate/setup");
  }

  const workspaceId = await getActiveWorkspaceId();

  let dashboard: DashboardData | null = null;
  if (workspaceId) {
    dashboard = await getDashboard(workspaceId, { cache: "no-store" }).catch(() => null);
  }

  return <OrchestratePageClient initialDashboard={dashboard} />;
}
