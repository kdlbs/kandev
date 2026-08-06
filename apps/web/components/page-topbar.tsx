import Link from "@/components/routing/app-link";
import { forwardRef } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
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
import { AppStatusDrawerTrigger } from "@/components/app-status-bar/app-status-surface-provider";
import { linkToTaskOverview } from "@/lib/links";

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
   * omitted the breadcrumb is two segments wide. `phoneOnlyLink` keeps the crumb
   * clickable below `md` but renders static text on desktop — for a crumb whose
   * target only resolves to somewhere new on a phone.
   */
  parents?: Array<{ label: string; href: string; phoneOnlyLink?: boolean }>;
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
  /** Phone-only Status entry point. Turn off where native route chrome owns it. */
  showStatusTrigger?: boolean;
  /**
   * When the breadcrumb shows a home crumb: "phone" hides it from `md` up
   * (where the AppSidebar owns Home), "always" keeps it at every width (for
   * surfaces whose sidebar swapped Home out), "none" suppresses it. Root-level
   * pages compute this via `useHomeAffordance`; direct callers keep the
   * default.
   */
  homeAffordance?: "none" | "phone" | "always";
  /** Where the home crumb lands (default: the workspace-less task overview). */
  homeHref?: string;
  /** Optional `data-testid` on the header element. */
  testId?: string;
};

// A middle crumb. `phoneOnlyLink` exists for a crumb that cannot navigate
// anywhere on desktop: /settings hands off to the remembered settings page, so
// on desktop the link lands back where it started. Rendering it as a link there
// makes the unsaved-changes guard offer to discard and leave, then not leave.
function ParentCrumb({
  crumb,
}: {
  crumb: { label: string; href: string; phoneOnlyLink?: boolean };
}) {
  const linkClass = "cursor-pointer text-muted-foreground hover:text-foreground transition-colors";
  if (!crumb.phoneOnlyLink) {
    return (
      <BreadcrumbLink asChild>
        <Link href={crumb.href} className={linkClass}>
          {crumb.label}
        </Link>
      </BreadcrumbLink>
    );
  }
  return (
    <>
      <BreadcrumbLink asChild className="md:hidden">
        <Link href={crumb.href} className={linkClass}>
          {crumb.label}
        </Link>
      </BreadcrumbLink>
      <span className="hidden text-muted-foreground md:inline">{crumb.label}</span>
    </>
  );
}

function BackLink({
  href,
  label,
  isHome = href === "/",
}: {
  href: string;
  label: string;
  isHome?: boolean;
}) {
  if (isHome) {
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
  homeAffordance,
  homeHref,
}: {
  backHref: string;
  backLabel: string;
  parents: Array<{ label: string; href: string; phoneOnlyLink?: boolean }> | undefined;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  homeAffordance: "none" | "phone" | "always";
  homeHref: string;
}) {
  const { t } = useTranslation();
  // The Home prefix is redundant on desktop, where the AppSidebar always shows
  // a Home nav item. Only render the back link when a page sets a non-root
  // backHref (e.g. a true ancestor route within a section).
  const showBackLink = backHref !== "/" && !!backLabel;
  // Below md the sidebar is hidden, so a root-level page would otherwise strand
  // phone users with no way back. "always" covers desktop states where the
  // sidebar swapped its Home row out (the settings tree takeover).
  const showHomeCrumb = !showBackLink && homeAffordance !== "none";
  const homeCrumbClass = cn("shrink-0", homeAffordance === "phone" && "md:hidden");
  return (
    <Breadcrumb
      className="relative z-10 min-w-0"
      aria-label={t("common:breadcrumb")}
      data-testid="page-topbar-breadcrumbs"
    >
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
        {showHomeCrumb && (
          <>
            <BreadcrumbItem className={homeCrumbClass} data-testid="topbar-phone-home">
              <BreadcrumbLink asChild>
                {/* The crumb is icon-only, so this label is its accessible
                    name — same catalog key the nav manifest's Home row uses. */}
                <BackLink href={homeHref} label={t("sidebar:home")} isHome />
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className={homeCrumbClass} />
          </>
        )}
        {parents?.flatMap((p) => [
          <BreadcrumbItem key={`${p.href}-item`} className="shrink-0">
            <ParentCrumb crumb={p} />
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
  homeAffordance: "none" | "phone" | "always";
  homeHref: string;
};

function TopbarLeading({
  variant,
  backHref,
  backLabel,
  parents,
  title,
  subtitle,
  icon,
  homeAffordance,
  homeHref,
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
      homeAffordance={homeAffordance}
      homeHref={homeHref}
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
    showStatusTrigger = true,
    homeAffordance = "phone",
    homeHref,
    testId,
  },
  ref,
) {
  return (
    <header
      ref={ref}
      data-testid={testId}
      className={cn(
        "relative flex h-10 min-h-11 shrink-0 items-center gap-3 border-b px-3 py-1 md:min-h-10",
        className,
      )}
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
        homeAffordance={homeAffordance}
        homeHref={homeHref ?? linkToTaskOverview()}
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
      {showStatusTrigger ? (
        <AppStatusDrawerTrigger className={cn(actions ? "ml-1" : "ml-auto", "shrink-0")} />
      ) : null}
    </header>
  );
});
