"use client";

import { IconHome, IconInbox, IconMessageCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeInboxCount } from "@/lib/state/slices/office/selectors";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";
import { homeDestinationHref } from "@/lib/navigation/core-destinations";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

type AppSidebarPrimaryNavProps = {
  collapsed: boolean;
};

export function AppSidebarPrimaryNav({ collapsed }: AppSidebarPrimaryNavProps) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const inboxCount = useAppStore(selectOfficeInboxCount);
  const mode = useOfficeModeState();
  const inOffice = mode === "office";
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const { activity: quickChatActivity, label: quickChatLabel } = useQuickChatActivity(workspaceId);
  const homeHref = mode === "unknown" ? undefined : homeDestinationHref({ workspaceId, inOffice });

  return (
    <div className="flex flex-col gap-0.5">
      <AppSidebarNavItem
        icon={IconHome}
        label={t("sidebar:home")}
        // The same rule the brand link resolves through, so the two "home"
        // affordances in this header can never point at different URLs.
        href={homeHref}
        disabled={mode === "unknown"}
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
          label={quickChatLabel}
          onClick={handleOpenQuickChat}
          collapsed={collapsed}
          activity={quickChatActivity}
        />
      )}
      <AppSidebarNewTaskItem collapsed={collapsed} />
    </div>
  );
}
