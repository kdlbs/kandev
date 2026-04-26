"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconSearch } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Input } from "@kandev/ui/input";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import type { InboxItem } from "@/lib/state/slices/orchestrate/types";
import { InboxItemRow } from "./inbox-item-row";

type TabValue = "mine" | "recent" | "all";

export default function InboxPage() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setInboxItems = useAppStore((s) => s.setInboxItems);
  const setInboxCount = useAppStore((s) => s.setInboxCount);
  const inboxItems = useAppStore((s) => s.orchestrate.inboxItems);
  const [tab, setTab] = useState<TabValue>("mine");
  const [search, setSearch] = useState("");

  const fetchInbox = useCallback(async () => {
    if (!workspaceId) return;
    const [itemsRes, countRes] = await Promise.all([
      orchestrateApi.getInbox(workspaceId),
      orchestrateApi.getInboxCount(workspaceId),
    ]);
    setInboxItems(itemsRes.items ?? []);
    setInboxCount(countRes.count ?? 0);
  }, [workspaceId, setInboxItems, setInboxCount]);

  useEffect(() => {
    void fetchInbox();
  }, [fetchInbox]);

  const filteredItems = useMemo(() => {
    let items: InboxItem[] = inboxItems;

    if (tab === "mine") {
      items = items.filter((i) => i.type === "approval" && i.status === "pending");
    } else if (tab === "recent") {
      items = items.slice(0, 20);
    }

    if (search) {
      const lower = search.toLowerCase();
      items = items.filter(
        (i) =>
          i.title.toLowerCase().includes(lower) ||
          (i.description?.toLowerCase().includes(lower) ?? false),
      );
    }
    return items;
  }, [inboxItems, tab, search]);

  const handleApprove = useCallback(
    async (id: string) => {
      await orchestrateApi.decideApproval(id, { status: "approved" });
      void fetchInbox();
    },
    [fetchInbox],
  );

  const handleReject = useCallback(
    async (id: string) => {
      await orchestrateApi.decideApproval(id, { status: "rejected" });
      void fetchInbox();
    },
    [fetchInbox],
  );

  return (
    <div className="space-y-4 p-6">
      <Tabs value={tab} onValueChange={(v) => setTab(v as TabValue)}>
        <div className="flex items-center justify-between">
          <TabsList>
            <TabsTrigger value="mine" className="cursor-pointer">
              Mine
            </TabsTrigger>
            <TabsTrigger value="recent" className="cursor-pointer">
              Recent
            </TabsTrigger>
            <TabsTrigger value="all" className="cursor-pointer">
              All
            </TabsTrigger>
          </TabsList>
          <div className="relative">
            <IconSearch className="absolute left-2.5 top-2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search..."
              className="w-[220px] h-8 pl-8 text-xs"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </div>
      </Tabs>

      <div className="border border-border rounded-lg divide-y divide-border">
        {filteredItems.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-muted-foreground">
            No inbox items
          </div>
        ) : (
          filteredItems.map((item) => (
            <InboxItemRow
              key={item.id}
              item={item}
              onApprove={handleApprove}
              onReject={handleReject}
            />
          ))
        )}
      </div>
    </div>
  );
}
