"use client";

import { useMemo } from "react";
import {
  IconMessage,
  IconListCheck,
  IconGitBranch,
  IconFolder,
  IconTerminal2,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Badge } from "@kandev/ui/badge";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";

type SessionMobileBottomNavProps = {
  activePanel: MobileSessionPanel;
  onPanelChange: (panel: MobileSessionPanel) => void;
  planBadge?: boolean;
  changesBadge?: number;
};

type NavItem = {
  id: MobileSessionPanel;
  label: string;
  icon: React.ReactNode;
  badge?: React.ReactNode;
};

export function SessionMobileBottomNav({
  activePanel,
  onPanelChange,
  planBadge = false,
  changesBadge = 0,
}: SessionMobileBottomNavProps) {
  const items: NavItem[] = useMemo(
    () => [
      {
        id: "chat",
        label: "Chat",
        icon: <IconMessage className="h-5 w-5" />,
      },
      {
        id: "plan",
        label: "Plan",
        icon: <IconListCheck className="h-5 w-5" />,
        badge: planBadge ? (
          <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-amber-500" />
        ) : undefined,
      },
      {
        id: "changes",
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
        id: "files",
        label: "Files",
        icon: <IconFolder className="h-5 w-5" />,
      },
      {
        id: "terminal",
        label: "Terminal",
        icon: <IconTerminal2 className="h-5 w-5" />,
      },
    ],
    [planBadge, changesBadge],
  );

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 flex items-center justify-around border-t border-border bg-background"
      style={{ paddingBottom: "env(safe-area-inset-bottom, 0px)" }}
    >
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => onPanelChange(item.id)}
          className={cn(
            "flex flex-col items-center justify-center py-2 px-3 gap-0.5 min-w-0 flex-1 transition-colors",
            activePanel === item.id
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
