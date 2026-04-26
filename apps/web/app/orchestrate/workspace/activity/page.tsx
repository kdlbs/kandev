import { listActivity } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../../lib/get-active-workspace";
import { ActivityPageClient } from "./activity-page-client";
import type { ActivityEntry } from "@/lib/state/slices/orchestrate/types";

export default async function ActivityPage() {
  const workspaceId = await getActiveWorkspaceId();

  let activity: ActivityEntry[] = [];
  if (workspaceId) {
    const res = await listActivity(workspaceId, undefined, { cache: "no-store" }).catch(() => ({
      activity: [],
    }));
    activity = res.activity ?? [];
  }

  return <ActivityPageClient initialActivity={activity} />;
}
