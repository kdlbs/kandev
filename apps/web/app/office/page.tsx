import { redirect } from "next/navigation";
import { getDashboard, getOnboardingState } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "./lib/get-active-workspace";
import { OfficePageClient } from "./page-client";
import type { DashboardData } from "@/lib/state/slices/office/types";

export default async function OfficePage() {
  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: true,
  }));
  if (!onboarding.completed) {
    redirect("/office/setup");
  }

  const workspaceId = await getActiveWorkspaceId();

  let dashboard: DashboardData | null = null;
  if (workspaceId) {
    dashboard = await getDashboard(workspaceId, { cache: "no-store" }).catch(() => null);
  }

  return <OfficePageClient initialDashboard={dashboard} />;
}
