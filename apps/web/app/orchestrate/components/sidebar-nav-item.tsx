"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";

type SidebarNavItemProps = {
  icon: TablerIcon;
  label: string;
  href: string;
  badge?: number;
  liveCount?: number;
  onClick?: () => void;
};

export function SidebarNavItem({ icon: Icon, label, href, badge, liveCount, onClick }: SidebarNavItemProps) {
  const pathname = usePathname();
  const isActive = pathname === href || (href !== "/orchestrate" && pathname.startsWith(href + "/"));

  const content = (
    <>
      <Icon className="h-4 w-4 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
      {typeof liveCount === "number" && liveCount > 0 && (
        <span className="flex items-center gap-1">
          <span className="h-1.5 w-1.5 rounded-full bg-blue-500 animate-pulse" />
          <span className="text-[11px] text-blue-600 dark:text-blue-400">{liveCount}</span>
        </span>
      )}
      {typeof badge === "number" && badge > 0 && (
        <Badge className="rounded-full px-1.5 py-0.5 text-xs bg-primary text-primary-foreground">
          {badge}
        </Badge>
      )}
    </>
  );

  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={cn(
          "flex items-center gap-2.5 px-3 py-2 text-[13px] font-medium rounded-md cursor-pointer w-full text-left",
          "text-foreground/80 hover:bg-accent/50",
        )}
      >
        {content}
      </button>
    );
  }

  return (
    <Link
      href={href}
      className={cn(
        "flex items-center gap-2.5 px-3 py-2 text-[13px] font-medium rounded-md cursor-pointer",
        isActive ? "bg-accent text-foreground" : "text-foreground/80 hover:bg-accent/50",
      )}
    >
      {content}
    </Link>
  );
}
