import Link from "next/link";
import { forwardRef } from "react";
import type { ReactNode } from "react";
import { IconArrowLeft, IconHome } from "@tabler/icons-react";
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
  /** Page title shown as the rightmost (current) breadcrumb */
  title: string;
  /** Optional subtitle shown to the right of the title */
  subtitle?: string;
  /** Optional icon rendered before the title */
  icon?: ReactNode;
  /** Where the back link navigates to (default: "/") */
  backHref?: string;
  /** Label for the parent breadcrumb (default: "Kandev") */
  backLabel?: string;
  /**
   * Optional middle crumbs inserted between the back link and the title — use
   * for nested sections (e.g. Home > Settings > Automations > New). When
   * omitted the breadcrumb is two segments wide.
   */
  parents?: Array<{ label: string; href: string }>;
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

function BackLink({ href, label }: { href: string; label: string }) {
  if (href === "/") {
    return (
      <Link
        href={href}
        aria-label={label}
        className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
      >
        <IconHome className="h-4 w-4" />
      </Link>
    );
  }
  return (
    <Link href={href} className="flex items-center gap-1.5 cursor-pointer">
      <IconArrowLeft className="h-3.5 w-3.5" />
      {label}
    </Link>
  );
}

function TopbarBreadcrumb({
  backHref,
  backLabel,
  parents,
  title,
  subtitle,
  icon,
}: {
  backHref: string;
  backLabel: string;
  parents: Array<{ label: string; href: string }> | undefined;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
}) {
  // The Home prefix is redundant now that the AppSidebar always shows a Home
  // nav item. Only render the back link when a page sets a non-root backHref
  // (e.g. a true ancestor route within a section).
  const showBackLink = backHref !== "/" && !!backLabel;
  return (
    <Breadcrumb className="relative z-10 min-w-0">
      <BreadcrumbList className="flex-nowrap text-sm">
        {showBackLink && (
          <>
            <BreadcrumbItem className="shrink-0">
              <BreadcrumbLink asChild>
                <BackLink href={backHref} label={backLabel} />
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className="shrink-0" />
          </>
        )}
        {parents?.flatMap((p) => [
          <BreadcrumbItem key={`${p.href}-item`} className="shrink-0">
            <BreadcrumbLink asChild>
              <Link
                href={p.href}
                className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
              >
                {p.label}
              </Link>
            </BreadcrumbLink>
          </BreadcrumbItem>,
          <BreadcrumbSeparator key={`${p.href}-sep`} className="shrink-0" />,
        ])}
        <BreadcrumbItem className="min-w-0">
          <BreadcrumbPage className="flex min-w-0 items-center gap-2">
            {icon}
            <span className="truncate text-sm font-medium">{title}</span>
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
  );
}

type TopbarLeadingProps = {
  variant: "breadcrumb" | "root";
  backHref: string;
  backLabel: string;
  parents: Array<{ label: string; href: string }> | undefined;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
};

function TopbarLeading({
  variant,
  backHref,
  backLabel,
  parents,
  title,
  subtitle,
  icon,
}: TopbarLeadingProps) {
  if (variant === "root") {
    if (!backLabel) return null;
    return (
      <div className="relative z-10 flex min-w-0 items-center">
        <span className="truncate text-[15px] font-semibold leading-none">{backLabel}</span>
      </div>
    );
  }
  return (
    <TopbarBreadcrumb
      backHref={backHref}
      backLabel={backLabel}
      parents={parents}
      title={title}
      subtitle={subtitle}
      icon={icon}
    />
  );
}

export const PageTopbar = forwardRef<HTMLElement, PageTopbarProps>(function PageTopbar(
  {
    title,
    subtitle,
    icon,
    backHref = "/",
    backLabel = "Kandev",
    parents,
    leading,
    center,
    leftActions,
    actions,
    variant = "breadcrumb",
    className,
    centerClassName,
    actionsClassName,
  },
  ref,
) {
  return (
    <header
      ref={ref}
      className={cn("relative flex h-10 shrink-0 items-center gap-3 border-b px-3 py-1", className)}
    >
      {leading}
      <TopbarLeading
        variant={variant}
        backHref={backHref}
        backLabel={backLabel}
        parents={parents}
        title={title}
        subtitle={subtitle}
        icon={icon}
      />
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
});
