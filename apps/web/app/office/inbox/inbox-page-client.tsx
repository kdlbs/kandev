"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconSearch } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Input } from "@kandev/ui/input";
import { toast } from "@/lib/toast/sonner";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { selectOfficeInboxItems } from "@/lib/state/slices/office/selectors";
import * as officeApi from "@/lib/api/domains/office-api";
import { loadOfficeAgents, loadOfficeInbox } from "@/hooks/use-office-workspace-data";
import type { InboxItem } from "@/lib/state/slices/office/types";
import { InboxItemRow } from "./inbox-item-row";
import { useTranslation } from "react-i18next";

type TabValue = "mine" | "recent" | "all";

type InboxPageClientProps = {
  initialItems: InboxItem[];
  initialCount: number;
  initialWorkspaceId?: string | null;
};

function useInboxData(workspaceId: string | null) {
  const store = useAppStoreApi();
  return useCallback(async () => {
    if (!workspaceId) return;
    // Mark-fixed changes both the inbox row and the agent pause state. Refresh
    // both workspace-owned caches through the shared loaders.
    await Promise.all([
      loadOfficeInbox(store, workspaceId, { cache: "no-store" }),
      loadOfficeAgents(store, workspaceId, { cache: "no-store" }),
    ]);
  }, [store, workspaceId]);
}

function useInitialInboxHydration(
  workspaceId: string | null,
  initialWorkspaceId: string | null | undefined,
  initialItems: InboxItem[],
  initialCount: number,
) {
  const setInboxItems = useAppStore((s) => s.setInboxItems);
  const setInboxCount = useAppStore((s) => s.setInboxCount);

  // Hydrate the SSR payload exactly once: it belongs to the workspace that was
  // active at SSR time, and re-running on a workspace switch would file it
  // under the new workspace.
  const initialHydratedRef = useRef(false);
  useEffect(() => {
    if (
      initialHydratedRef.current ||
      !workspaceId ||
      (initialWorkspaceId !== undefined && initialWorkspaceId !== workspaceId)
    ) {
      return;
    }
    initialHydratedRef.current = true;
    if (initialItems.length > 0) setInboxItems(workspaceId, initialItems);
    if (initialCount > 0) setInboxCount(workspaceId, initialCount);
  }, [initialCount, initialItems, initialWorkspaceId, setInboxCount, setInboxItems, workspaceId]);
}

function useApprovalActions(fetchInbox: () => Promise<void>) {
  const { t } = useTranslation();
  const handleApprove = useCallback(
    async (id: string) => {
      try {
        await officeApi.decideApproval(id, { status: "approved" });
        void fetchInbox();
        toast.success(t("office:approved"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToApprove"));
      }
    },
    [fetchInbox, t],
  );

  const handleReject = useCallback(
    async (id: string) => {
      try {
        await officeApi.decideApproval(id, { status: "rejected" });
        void fetchInbox();
        toast.success(t("office:rejected"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToReject"));
      }
    },
    [fetchInbox, t],
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
  const { t } = useTranslation();
  return (
    <Tabs value={tab} onValueChange={(v) => onTabChange(v as TabValue)}>
      <div className="flex items-center justify-between">
        <TabsList>
          <TabsTrigger value="mine" className="cursor-pointer">
            {t("office:mine")}
          </TabsTrigger>
          <TabsTrigger value="recent" className="cursor-pointer">
            {t("office:recent")}
          </TabsTrigger>
          <TabsTrigger value="all" className="cursor-pointer">
            {t("office:all")}
          </TabsTrigger>
        </TabsList>
        <div className="relative">
          <IconSearch className="absolute left-2.5 top-2 h-3.5 w-3.5 text-muted-foreground" />
          <Input
            placeholder={t("office:search")}
            className="w-[220px] h-8 pl-8 text-xs"
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
          />
        </div>
      </div>
    </Tabs>
  );
}

export function InboxPageClient({
  initialItems,
  initialCount,
  initialWorkspaceId,
}: InboxPageClientProps) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inboxItems = useAppStore(selectOfficeInboxItems);
  const [tab, setTab] = useState<TabValue>("mine");
  const [search, setSearch] = useState("");

  useInitialInboxHydration(workspaceId, initialWorkspaceId, initialItems, initialCount);
  const fetchInbox = useInboxData(workspaceId);
  const { handleApprove, handleReject } = useApprovalActions(fetchInbox);

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
            <p className="text-sm text-muted-foreground">{t("office:allClear")}</p>
            <p className="text-xs text-muted-foreground mt-1">
              {t("office:approvalsAlertsAndItemsNeedingYour")}
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
