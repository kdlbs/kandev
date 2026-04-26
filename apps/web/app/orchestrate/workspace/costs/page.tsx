import { getCosts } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../../lib/get-active-workspace";
import { CostsPageClient } from "./costs-page-client";
import type { CostSummary } from "@/lib/state/slices/orchestrate/types";

export default async function CostsPage() {
  const workspaceId = await getActiveWorkspaceId();

  let costSummary: CostSummary | null = null;
  if (workspaceId) {
    costSummary = await getCosts(workspaceId, { cache: "no-store" }).catch(() => null);
  }

  return <CostsPageClient initialCostSummary={costSummary} />;
}
