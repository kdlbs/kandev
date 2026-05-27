"use client";

import { useState } from "react";
import { IconHome, IconInbox, IconMessageCircle, IconSquarePlus } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { NewTaskDialog } from "@/app/office/components/new-task-dialog";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type AppSidebarPrimaryNavProps = {
  collapsed: boolean;
};

export function AppSidebarPrimaryNav({ collapsed }: AppSidebarPrimaryNavProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inboxCount = useAppStore((s) => s.office.inboxCount);
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const [newTaskOpen, setNewTaskOpen] = useState(false);

  return (
    <div className="flex flex-col gap-0.5">
      <AppSidebarNavItem icon={IconHome} label="Home" href="/" collapsed={collapsed} exactMatch />
      <AppSidebarNavItem
        icon={IconInbox}
        label="Inbox"
        href="/office/inbox"
        badge={inboxCount}
        collapsed={collapsed}
      />
      {workspaceId && (
        <AppSidebarNavItem
          icon={IconMessageCircle}
          label="Quick Chat"
          onClick={handleOpenQuickChat}
          collapsed={collapsed}
        />
      )}
      <AppSidebarNavItem
        icon={IconSquarePlus}
        label="New Task"
        onClick={() => setNewTaskOpen(true)}
        collapsed={collapsed}
      />
      <NewTaskDialog open={newTaskOpen} onOpenChange={setNewTaskOpen} />
    </div>
  );
}
