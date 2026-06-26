"use client";

import { useCallback, useMemo, useState } from "react";
import { IconSearch } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Input } from "@kandev/ui/input";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { useOfficeInboxData } from "@/hooks/domains/office/use-office-data";
import { decideApproval } from "@/lib/api/domains/office-api";
import type { InboxItem } from "@/lib/state/slices/office/types";
import { InboxItemRow } from "./inbox-item-row";

type TabValue = "mine" | "recent" | "all";

type InboxPageClientProps = {
  initialItems: InboxItem[];
  initialCount: number;
};

function useApprovalActions(fetchInbox: () => Promise<void>) {
  const handleApprove = useCallback(
    async (id: string) => {
      try {
        await decideApproval(id, { status: "approved" });
        void fetchInbox();
        toast.success("Approved");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to approve");
      }
    },
    [fetchInbox],
  );

  const handleReject = useCallback(
    async (id: string) => {
      try {
        await decideApproval(id, { status: "rejected" });
        void fetchInbox();
        toast.success("Rejected");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to reject");
      }
    },
    [fetchInbox],
  );

  return { handleApprove, handleReject };
}

function InboxToolbar({
  tab,
  search,
  onTabChange,
  onSearchChange,
}: {
  tab: TabValue;
  search: string;
  onTabChange: (v: TabValue) => void;
  onSearchChange: (v: string) => void;
}) {
  return (
    <Tabs value={tab} onValueChange={(v) => onTabChange(v as TabValue)}>
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
            onChange={(e) => onSearchChange(e.target.value)}
          />
        </div>
      </div>
    </Tabs>
  );
}

export function InboxPageClient({ initialItems, initialCount }: InboxPageClientProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [tab, setTab] = useState<TabValue>("mine");
  const [search, setSearch] = useState("");

  const inboxQuery = useOfficeInboxData(workspaceId, initialItems, initialCount);
  const fetchInbox = inboxQuery.refetchAll;
  const { handleApprove, handleReject } = useApprovalActions(fetchInbox);
  const inboxItems = inboxQuery.data?.items ?? initialItems;

  const filteredItems = useMemo(() => {
    let items: InboxItem[] = inboxItems;
    if (tab === "mine") {
      items = items.filter(
        (i) =>
          (i.type === "approval" && i.status === "pending") ||
          i.type === "task_review_request" ||
          i.type === "agent_run_failed" ||
          i.type === "agent_paused_after_failures",
      );
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

  return (
    <div className="space-y-4 p-6">
      <InboxToolbar tab={tab} search={search} onTabChange={setTab} onSearchChange={setSearch} />
      <div className="border border-border rounded-lg divide-y divide-border overflow-hidden">
        {filteredItems.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <p className="text-sm text-muted-foreground">All clear.</p>
            <p className="text-xs text-muted-foreground mt-1">
              Approvals, alerts, and items needing your attention appear here.
            </p>
          </div>
        ) : (
          filteredItems.map((item) => (
            <InboxItemRow
              key={item.id}
              item={item}
              onApprove={handleApprove}
              onReject={handleReject}
              onChanged={() => void fetchInbox()}
            />
          ))
        )}
      </div>
    </div>
  );
}
