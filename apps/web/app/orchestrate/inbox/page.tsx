import { getInbox, getInboxCount } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { InboxPageClient } from "./inbox-page-client";
import type { InboxItem } from "@/lib/state/slices/orchestrate/types";

export default async function InboxPage() {
  const workspaceId = await getActiveWorkspaceId();

  let items: InboxItem[] = [];
  let count = 0;
  if (workspaceId) {
    const [itemsRes, countRes] = await Promise.all([
      getInbox(workspaceId, { cache: "no-store" }).catch(() => ({ items: [] })),
      getInboxCount(workspaceId, { cache: "no-store" }).catch(() => ({ count: 0 })),
    ]);
    items = itemsRes.items ?? [];
    count = countRes.count ?? 0;
  }

  return <InboxPageClient initialItems={items} initialCount={count} />;
}
