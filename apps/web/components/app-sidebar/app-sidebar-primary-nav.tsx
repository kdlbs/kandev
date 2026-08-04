"use client";

import { IconHome, IconInbox, IconMessageCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { linkToTaskOverview } from "@/lib/links";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

type AppSidebarPrimaryNavProps = {
  collapsed: boolean;
};

export function AppSidebarPrimaryNav({ collapsed }: AppSidebarPrimaryNavProps) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inboxCount = useAppStore((s) => s.office.inboxCount);
  const inOffice = useInOffice();
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  return (
    <div className="flex flex-col gap-0.5">
      <AppSidebarNavItem
        icon={IconHome}
        label={t("sidebar:home")}
        href={inOffice ? "/office" : linkToTaskOverview({ workspaceId: workspaceId ?? undefined })}
        collapsed={collapsed}
        exactMatch
      />
      {inOffice && (
        <AppSidebarNavItem
          icon={IconInbox}
          label={t("sidebar:inbox")}
          href="/office/inbox"
          badge={inboxCount}
          collapsed={collapsed}
        />
      )}
      {workspaceId && collapsed && (
        <AppSidebarNavItem
          icon={IconMessageCircle}
          label={t("sidebar:quickChat")}
          onClick={handleOpenQuickChat}
          collapsed={collapsed}
        />
      )}
      <AppSidebarNewTaskItem collapsed={collapsed} />
    </div>
  );
}
