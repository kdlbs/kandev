"use client";

import { IconHome, IconInbox, IconMessageCircle } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { officeInboxQueryOptions } from "@/lib/query/query-options/office";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

type AppSidebarPrimaryNavProps = {
  collapsed: boolean;
};

export function AppSidebarPrimaryNav({ collapsed }: AppSidebarPrimaryNavProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inOffice = useInOffice();
  const inboxQuery = useQuery(officeInboxQueryOptions(inOffice ? (workspaceId ?? "") : ""));
  const inboxItems = inboxQuery.data?.items ?? [];
  const inboxCount = inboxQuery.data?.total_count ?? inboxItems.length;
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  return (
    <div className="flex flex-col gap-0.5">
      <AppSidebarNavItem
        icon={IconHome}
        label="Home"
        href={inOffice ? "/office" : "/"}
        collapsed={collapsed}
        exactMatch
      />
      {inOffice && (
        <AppSidebarNavItem
          icon={IconInbox}
          label="Inbox"
          href="/office/inbox"
          badge={inboxCount}
          collapsed={collapsed}
        />
      )}
      {workspaceId && collapsed && (
        <AppSidebarNavItem
          icon={IconMessageCircle}
          label="Quick Chat"
          onClick={handleOpenQuickChat}
          collapsed={collapsed}
        />
      )}
      <AppSidebarNewTaskItem collapsed={collapsed} />
    </div>
  );
}
