"use client";

import { useMemo } from "react";
import {
  IconMessage,
  IconListCheck,
  IconGitBranch,
  IconFolder,
  IconTerminal2,
  IconGitMerge,
  IconActivity,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";

type SessionMobileBottomNavProps = {
  activePanel: MobileSessionPanel;
  onPanelChange: (panel: MobileSessionPanel) => void;
  planBadge?: boolean;
  changesBadge?: number;
  hasReview?: boolean;
  showStatus: boolean;
  onOpenStatus: () => void;
};

type NavItem = {
  label: string;
  icon: React.ReactNode;
  badge?: React.ReactNode;
} & ({ panel: MobileSessionPanel; onClick?: never } | { panel?: never; onClick: () => void });

export function SessionMobileBottomNav({
  activePanel,
  onPanelChange,
  planBadge = false,
  changesBadge = 0,
  hasReview = false,
  showStatus,
  onOpenStatus,
}: SessionMobileBottomNavProps) {
  const items: NavItem[] = useMemo(
    () => [
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
      ...(showStatus
        ? [
            {
              label: "Status",
              icon: <IconActivity className="h-5 w-5" />,
              onClick: onOpenStatus,
            },
          ]
        : []),
    ],
    [planBadge, changesBadge, hasReview, showStatus, onOpenStatus],
  );

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 flex items-center justify-around border-t border-border bg-background"
      style={{ paddingBottom: "env(safe-area-inset-bottom, 0px)" }}
    >
      {items.map((item) => (
        <button
          key={item.label}
          type="button"
          onClick={item.onClick ?? (() => onPanelChange(item.panel))}
          className={cn(
            "flex min-h-11 flex-col items-center justify-center py-2 px-3 gap-0.5 min-w-0 flex-1 transition-colors",
            activePanel === item.panel
              ? "text-primary"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          <span className="relative">
            {item.icon}
            {item.badge}
          </span>
          <span className="text-[10px] font-medium truncate">{item.label}</span>
        </button>
      ))}
    </nav>
  );
}
