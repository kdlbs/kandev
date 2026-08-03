"use client";

import { IconLogout } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Avatar, AvatarFallback } from "@kandev/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { logout } from "@/lib/api/domains/auth-api";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

function initialsFor(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return `${parts[0]![0]}${parts[parts.length - 1]![0]}`.toUpperCase();
}

async function handleLogout() {
  try {
    await logout();
  } finally {
    window.location.assign("/login");
  }
}

/**
 * Shows the current logged-in user in the sidebar footer with a logout
 * action. Only meaningful in multi-user "enabled" auth mode; the caller is
 * responsible for gating on mode/user presence so single-user (disabled)
 * installs never render this.
 */
export function CurrentUserChip({
  collapsed,
  className,
}: {
  collapsed: boolean;
  className?: string;
}) {
  const { t } = useTranslation();
  const user = useAppStore((s) => s.auth.user);
  if (!user) return null;

  const label = user.display_name || user.email;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          data-testid="current-user-chip"
          className={cn(
            "flex items-center gap-1.5 rounded-md cursor-pointer overflow-hidden border border-transparent transition-colors hover:border-border hover:bg-muted/50",
            collapsed ? "justify-center p-1" : "px-1.5 py-1 max-w-[140px]",
            className,
          )}
        >
          <Avatar size="sm">
            <AvatarFallback className="text-[10px]">{initialsFor(label)}</AvatarFallback>
          </Avatar>
          {!collapsed && <span className="truncate text-xs">{label}</span>}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={collapsed ? "center" : "start"} side="top">
        <DropdownMenuItem
          data-testid="current-user-logout"
          onClick={() => void handleLogout()}
          className="cursor-pointer"
        >
          <IconLogout className="h-4 w-4" />
          {t("sidebar:logOut")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
