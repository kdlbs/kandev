import Link from "next/link";
import type { ReactNode } from "react";
import { IconArrowLeft } from "@tabler/icons-react";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import { cn } from "@kandev/ui/lib/utils";

type PageTopbarProps = {
  /** Page title shown in the breadcrumb */
  title: string;
  /** Optional subtitle shown to the right of the title */
  subtitle?: string;
  /** Optional icon rendered before the title */
  icon?: ReactNode;
  /** Where the back link navigates to (default: "/") */
  backHref?: string;
  /** Label for the parent breadcrumb (default: "KanDev") */
  backLabel?: string;
  /** Optional content rendered before the breadcrumb */
  leading?: ReactNode;
  /** Optional content rendered at the visual center of the topbar */
  center?: ReactNode;
  /** Optional content rendered alongside the left orientation label */
  leftActions?: ReactNode;
  /** Optional content rendered on the right side of the topbar */
  actions?: ReactNode;
  variant?: "breadcrumb" | "root";
  className?: string;
  centerClassName?: string;
  actionsClassName?: string;
};

export function PageTopbar({
  title,
  subtitle,
  icon,
  backHref = "/",
  backLabel = "KanDev",
  leading,
  center,
  leftActions,
  actions,
  variant = "breadcrumb",
  className,
  centerClassName,
  actionsClassName,
}: PageTopbarProps) {
  const rootLabel = backLabel;

  return (
    <header
      className={cn("relative flex h-14 shrink-0 items-center gap-3 border-b px-4", className)}
    >
      {leading}
      {variant === "root" ? (
        <div className="relative z-10 flex min-w-0 items-center">
          <span className="truncate text-sm font-semibold">{rootLabel}</span>
        </div>
      ) : (
        <Breadcrumb className="relative z-10 min-w-0">
          <BreadcrumbList className="flex-nowrap">
            <BreadcrumbItem className="shrink-0">
              <BreadcrumbLink asChild className="flex items-center gap-1.5 cursor-pointer">
                <Link href={backHref}>
                  <IconArrowLeft className="h-3.5 w-3.5" />
                  {backLabel}
                </Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className="shrink-0" />
            <BreadcrumbItem className="min-w-0">
              <BreadcrumbPage className="flex min-w-0 items-center gap-2">
                {icon}
                <span className="truncate text-sm font-semibold">{title}</span>
                {subtitle && (
                  <>
                    <span className="hidden text-muted-foreground/50 sm:inline">·</span>
                    <span className="hidden truncate text-xs text-muted-foreground sm:inline">
                      {subtitle}
                    </span>
                  </>
                )}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      )}
      {leftActions && (
        <div className="relative z-10 flex shrink-0 items-center gap-1 [&:empty]:hidden">
          {leftActions}
        </div>
      )}
      {center && (
        <div
          className={cn(
            "pointer-events-none absolute left-1/2 top-1/2 z-0 -translate-x-1/2 -translate-y-1/2",
            centerClassName,
          )}
        >
          <div className="pointer-events-auto">{center}</div>
        </div>
      )}
      {actions && (
        <div
          className={cn("relative z-10 ml-auto flex shrink-0 items-center gap-2", actionsClassName)}
        >
          {actions}
        </div>
      )}
    </header>
  );
}
