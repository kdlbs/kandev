"use client";

import { IconLogout } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Avatar, AvatarFallback } from "@kandev/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { logout } from "@/lib/api/domains/auth-api";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { initialsFor } from "@/lib/user-initials";

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
          aria-label={label}
          title={label}
          data-testid="current-user-chip"
          className={cn(
            "flex size-7 shrink-0 items-center justify-center rounded-md cursor-pointer border border-transparent transition-colors hover:border-border hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:size-11",
            className,
          )}
        >
          <Avatar size="sm">
            <AvatarFallback className="text-[10px]">{initialsFor(label)}</AvatarFallback>
          </Avatar>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={collapsed ? "center" : "start"} side="top">
        <DropdownMenuLabel className="max-w-60">
          <div className="truncate">{label}</div>
          {user.display_name && (
            <div className="truncate text-xs font-normal text-muted-foreground">{user.email}</div>
          )}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          data-testid="current-user-logout"
          onClick={() => void handleLogout()}
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
        >
          <IconLogout className="h-4 w-4" />
          {t("sidebar:logOut")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
