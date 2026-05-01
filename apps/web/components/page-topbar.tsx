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
  /** Optional content rendered on the right side of the topbar */
  actions?: ReactNode;
  className?: string;
  actionsClassName?: string;
};

export function PageTopbar({
  title,
  subtitle,
  icon,
  backHref = "/",
  backLabel = "KanDev",
  leading,
  actions,
  className,
  actionsClassName,
}: PageTopbarProps) {
  return (
    <header className={cn("flex h-14 shrink-0 items-center gap-3 border-b px-4", className)}>
      {leading}
      <Breadcrumb className="min-w-0">
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
      {actions && (
        <div className={cn("ml-auto flex shrink-0 items-center gap-2", actionsClassName)}>
          {actions}
        </div>
      )}
    </header>
  );
}
