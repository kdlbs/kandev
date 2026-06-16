import { redirect } from "next/navigation";
import { getDashboard, getOnboardingState } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "./lib/get-active-workspace";
import { OfficePageClient } from "./page-client";
import type { DashboardData } from "@/lib/state/slices/office/types";

type SearchParams = Promise<{ workspaceId?: string }>;

export default async function OfficePage({ searchParams }: { searchParams?: SearchParams }) {
  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: true,
  }));
  if (!onboarding.completed) {
    redirect("/office/setup");
  }

  const params = searchParams ? await searchParams : {};
  const workspaceId = await getActiveWorkspaceId(params.workspaceId);
  if (!workspaceId) {
    redirect("/office/setup?mode=new");
  }

  let dashboard: DashboardData | null = null;
  dashboard = await getDashboard(workspaceId, { cache: "no-store" }).catch(() => null);

  return <OfficePageClient initialDashboard={dashboard} />;
}
