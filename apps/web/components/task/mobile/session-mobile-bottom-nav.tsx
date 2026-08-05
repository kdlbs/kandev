"use client";

import { useMemo, useState } from "react";
import {
  IconMessage,
  IconListCheck,
  IconGitBranch,
  IconFolder,
  IconTerminal2,
  IconGitMerge,
  IconActivity,
  IconLayoutGrid,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import type { ConnectionIssueSeverity } from "@/lib/types/connection";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { parsePluginPanelId } from "@/lib/state/layout-manager/plugin-panels";
import { PluginPanelPicker } from "./plugin-panel-picker";

type SessionMobileBottomNavProps = {
  activePanel: MobileSessionPanel;
  onPanelChange: (panel: MobileSessionPanel) => void;
  planBadge?: boolean;
  changesBadge?: number;
  hasReview?: boolean;
  showStatus: boolean;
  onOpenStatus: () => void;
  connectionIssueSeverity?: ConnectionIssueSeverity;
};

type NavItem = {
  label: string;
  icon: React.ReactNode;
  badge?: React.ReactNode;
  active?: boolean;
  connectionIssueSeverity?: Exclude<ConnectionIssueSeverity, "none">;
} & ({ panel: MobileSessionPanel; onClick?: never } | { panel?: never; onClick: () => void });

function hasMobilePluginPanels(): boolean {
  return pluginRegistry.getTaskPanels().some((registration) => registration.mobileEnabled);
}

function buildMobileNavItems({
  activePanel,
  planBadge,
  changesBadge,
  hasReview,
  showStatus,
  onOpenStatus,
  onOpenPluginPicker,
  connectionIssueSeverity,
  t,
}: {
  activePanel: MobileSessionPanel;
  planBadge: boolean;
  changesBadge: number;
  hasReview: boolean;
  showStatus: boolean;
  onOpenStatus: () => void;
  onOpenPluginPicker: () => void;
  connectionIssueSeverity: ConnectionIssueSeverity;
  t: (key: string) => string;
}): NavItem[] {
  return [
    {
      panel: "chat",
      label: "Chat",
      icon: <IconMessage className="h-5 w-5" />,
    },
    {
      panel: "plan",
      label: "Plan",
      icon: <IconListCheck className="h-5 w-5" />,
      badge: planBadge ? (
        <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-amber-500" />
      ) : undefined,
    },
    {
      panel: "changes",
      label: "Changes",
      icon: <IconGitBranch className="h-5 w-5" />,
      badge:
        changesBadge > 0 ? (
          <Badge
            variant="secondary"
            className="absolute -top-1 -right-2 h-4 min-w-4 px-1 text-[10px]"
          >
            {changesBadge > 99 ? "99+" : changesBadge}
          </Badge>
        ) : undefined,
    },
    {
      panel: "files",
      label: "Files",
      icon: <IconFolder className="h-5 w-5" />,
    },
    ...(hasReview
      ? [
          {
            panel: "review" as const,
            label: "Review",
            icon: <IconGitMerge className="h-5 w-5" />,
          },
        ]
      : []),
    {
      panel: "terminal",
      label: "Terminal",
      icon: <IconTerminal2 className="h-5 w-5" />,
    },
    ...(hasMobilePluginPanels()
      ? [
          {
            label: t("common:panels"),
            icon: <IconLayoutGrid className="h-5 w-5" />,
            active: parsePluginPanelId(activePanel) !== undefined,
            onClick: onOpenPluginPicker,
          },
        ]
      : []),
    ...(showStatus
      ? [
          {
            label: "Status",
            icon: <IconActivity className="h-5 w-5" />,
            onClick: onOpenStatus,
            ...(connectionIssueSeverity !== "none" && { connectionIssueSeverity }),
          },
        ]
      : []),
  ];
}

export function SessionMobileBottomNav({
  activePanel,
  onPanelChange,
  planBadge = false,
  changesBadge = 0,
  hasReview = false,
  showStatus,
  onOpenStatus,
  connectionIssueSeverity = "none",
}: SessionMobileBottomNavProps) {
  const { t } = useTranslation();
  usePluginRegistry();
  const registryVersion = pluginRegistry.getVersion();
  const [pluginPickerOpen, setPluginPickerOpen] = useState(false);
  const items: NavItem[] = useMemo(
    () =>
      buildMobileNavItems({
        activePanel,
        planBadge,
        changesBadge,
        hasReview,
        showStatus,
        onOpenStatus,
        onOpenPluginPicker: () => setPluginPickerOpen(true),
        connectionIssueSeverity,
        t,
      }),
    [
      planBadge,
      changesBadge,
      hasReview,
      showStatus,
      onOpenStatus,
      connectionIssueSeverity,
      registryVersion,
      activePanel,
      t,
    ],
  );

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 flex items-center justify-around border-t border-border bg-background"
      style={{ paddingBottom: "env(safe-area-inset-bottom, 0px)" }}
    >
      {items.map((item) => (
        <MobileNavButton
          key={item.panel ?? item.label}
          item={item}
          activePanel={activePanel}
          onPanelChange={onPanelChange}
        />
      ))}
      <PluginPanelPicker
        open={pluginPickerOpen}
        onOpenChange={setPluginPickerOpen}
        onSelect={onPanelChange}
      />
    </nav>
  );
}

function MobileNavButton({
  item,
  activePanel,
  onPanelChange,
}: {
  item: NavItem;
  activePanel: MobileSessionPanel;
  onPanelChange: (panel: MobileSessionPanel) => void;
}) {
  const issueDetails = useConnectionIssueCopy(item.connectionIssueSeverity ?? "none");

  return (
    <button
      type="button"
      onClick={item.onClick ?? (() => onPanelChange(item.panel))}
      className={cn(
        "flex min-h-11 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 px-3 py-2 transition-colors",
        mobileNavColorClass(item, activePanel, issueDetails !== null),
      )}
      aria-label={issueDetails?.description}
      data-connection-severity={item.connectionIssueSeverity}
    >
      <span className="relative">
        {item.icon}
        {item.badge}
        {issueDetails && (
          <span
            className={cn(
              "absolute -right-1 -top-1 size-2 rounded-full ring-2 ring-background",
              issueDetails.dotClass,
            )}
            aria-hidden="true"
          />
        )}
      </span>
      <span className="text-[10px] font-medium truncate">{item.label}</span>
    </button>
  );
}

function mobileNavColorClass(
  item: NavItem,
  activePanel: MobileSessionPanel,
  hasConnectionIssue: boolean,
) {
  if (hasConnectionIssue) {
    return item.connectionIssueSeverity === "lost" ? "text-destructive" : "text-amber-500";
  }
  return activePanel === item.panel || item.active === true
    ? "text-primary"
    : "text-muted-foreground hover:text-foreground";
}
