"use client";

import { IconHome, IconInbox, IconMessageCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

type AppSidebarPrimaryNavProps = {
  collapsed: boolean;
};

export function AppSidebarPrimaryNav({ collapsed }: AppSidebarPrimaryNavProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inboxCount = useAppStore((s) => s.office.inboxCount);
  const officeEnabled = useFeature("office");
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  return (
    <div className="flex flex-col gap-0.5">
      <AppSidebarNavItem icon={IconHome} label="Home" href="/" collapsed={collapsed} exactMatch />
      {officeEnabled && (
        <AppSidebarNavItem
          icon={IconInbox}
          label="Inbox"
          href="/office/inbox"
          badge={inboxCount}
          collapsed={collapsed}
        />
      )}
      {workspaceId && (
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
