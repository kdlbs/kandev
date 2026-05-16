import { getInbox } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { InboxPageClient } from "./inbox-page-client";
import type { InboxItem } from "@/lib/state/slices/office/types";

export default async function InboxPage() {
  const workspaceId = await getActiveWorkspaceId();

  let items: InboxItem[] = [];
  let count = 0;
  if (workspaceId) {
    // Single round-trip: getInbox returns items + total_count
    // (Stream F of office optimization). Was 2 parallel calls.
    const res = await getInbox(workspaceId, { cache: "no-store" }).catch(() => ({
      items: [] as InboxItem[],
      total_count: 0,
    }));
    items = res.items ?? [];
    count = res.total_count ?? items.length;
  }

  return <InboxPageClient initialItems={items} initialCount={count} />;
}
