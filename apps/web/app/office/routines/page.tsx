import { listRoutines } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { RoutinesPageClient } from "./routines-page-client";
import type { Routine } from "@/lib/state/slices/office/types";

export default async function RoutinesPage() {
  const workspaceId = await getActiveWorkspaceId();

  let routines: Routine[] = [];
  if (workspaceId) {
    const res = await listRoutines(workspaceId, { cache: "no-store" }).catch(() => ({
      routines: [],
    }));
    routines = res.routines ?? [];
  }

  return <RoutinesPageClient initialRoutines={routines} />;
}
