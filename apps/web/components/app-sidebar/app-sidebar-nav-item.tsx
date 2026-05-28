"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { SIDEBAR_ITEM_ACTIVE, SIDEBAR_ITEM_INACTIVE } from "./app-sidebar-constants";

type AppSidebarNavItemProps = {
  icon: TablerIcon;
  label: string;
  href?: string;
  badge?: number;
  onClick?: () => void;
  collapsed: boolean;
  /** Override the auto-derived active-state from pathname. */
  isActive?: boolean;
  /** Suppress the default href-startsWith activation (use for "Home"). */
  exactMatch?: boolean;
  /** Render as visually disabled and ignore clicks. */
  disabled?: boolean;
};

type TriggerProps = {
  onClick?: () => void;
  disabled: boolean;
  baseClass: string;
  label: string;
  href?: string;
  inner: React.ReactNode;
};

function renderTrigger({ onClick, disabled, baseClass, label, href, inner }: TriggerProps) {
  if (onClick) {
    return (
      <button
        type="button"
        onClick={disabled ? undefined : onClick}
        className={baseClass}
        aria-label={label}
        aria-disabled={disabled || undefined}
        disabled={disabled}
      >
        {inner}
      </button>
    );
  }
  if (disabled) {
    return (
      <span className={baseClass} aria-label={label} aria-disabled="true">
        {inner}
      </span>
    );
  }
  return (
    <Link href={href ?? "#"} className={baseClass} aria-label={label}>
      {inner}
    </Link>
  );
}

function isPathActive(pathname: string, href: string | undefined, exactMatch: boolean): boolean {
  if (!href) return false;
  if (exactMatch) return pathname === href;
  if (pathname === href) return true;
  return href !== "/" && pathname.startsWith(`${href}/`);
}

export function AppSidebarNavItem({
  icon: Icon,
  label,
  href,
  badge,
  onClick,
  collapsed,
  isActive,
  exactMatch = false,
  disabled = false,
}: AppSidebarNavItemProps) {
  const pathname = usePathname();
  const active = isActive ?? isPathActive(pathname, href, exactMatch);

  const baseClass = cn(
    "flex items-center rounded-md text-[13px] font-medium transition-colors",
    collapsed ? "h-9 w-9 justify-center mx-auto" : "h-9 px-2.5 gap-2.5 w-full text-left",
    disabled
      ? "cursor-not-allowed text-foreground/40"
      : cn("cursor-pointer", active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE),
  );

  const inner = (
    <>
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && (
        <>
          <span className="flex-1 truncate sidebar-fade-in">{label}</span>
          {typeof badge === "number" && badge > 0 && (
            <Badge className="rounded-full px-1.5 py-0.5 text-xs bg-primary text-primary-foreground">
              {badge}
            </Badge>
          )}
        </>
      )}
    </>
  );

  const buttonOrLink = renderTrigger({ onClick, disabled, baseClass, label, href, inner });

  if (!collapsed) return buttonOrLink;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{buttonOrLink}</TooltipTrigger>
      <TooltipContent side="right">
        {label}
        {typeof badge === "number" && badge > 0 ? ` (${badge})` : ""}
      </TooltipContent>
    </Tooltip>
  );
}
