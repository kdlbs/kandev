import Link from "@/components/routing/app-link";
import { forwardRef, useRef } from "react";
import type { ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconChevronRight, IconDots, IconHome } from "@tabler/icons-react";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@kandev/ui/lib/utils";
import { AppStatusDrawerTrigger } from "@/components/app-status-bar/app-status-surface-provider";
import { useTopbarPressure } from "@/hooks/use-topbar-pressure";
import { linkToTaskOverview } from "@/lib/links";

/** The one bar height. Bespoke bars adopt this token instead of redeclaring it. */
export const TOPBAR_HEIGHT_CLASSNAME = "h-10 min-h-11 md:min-h-10";

/**
 * A middle breadcrumb between the leading crumb and the page title. No `href`
 * means orientation rather than navigation; `phoneOnlyLink` keeps the crumb
 * clickable below `md` (where the sidebar is hidden) and static above it.
 */
export type ParentCrumb = { label: string; href?: string; phoneOnlyLink?: boolean };

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
   * omitted the breadcrumb is two segments wide. Crumbs without an `href`
   * render as static text: orientation, not navigation. `phoneOnlyLink` keeps
   * the crumb clickable below `md` (where the sidebar is hidden) but renders
   * static text on desktop (e.g. "Settings", whose menu the sidebar owns).
   */
  parents?: ParentCrumb[];
  /**
   * Optional interactive replacement for the title crumb (e.g. an inline
   * rename control). `title` stays required: it is the accessible name and
   * the width the space policy measures.
   */
  titleSlot?: ReactNode;
  /** Optional content rendered before the breadcrumb */
  leading?: ReactNode;
  /** Optional content rendered between the breadcrumb and the actions */
  center?: ReactNode;
  /** Optional content rendered alongside the left orientation label */
  leftActions?: ReactNode;
  /** Optional content rendered on the right side of the topbar */
  actions?: ReactNode;
  /**
   * Actions that fold into a trailing "…" menu when even the collapsed
   * breadcrumb would squeeze the title below its floor. Rendered inline
   * (before `actions`) while there is room. Keep these self-contained;
   * they unmount and remount when the fold engages.
   */
  overflowActions?: ReactNode;
  className?: string;
  /**
   * Which zone absorbs the bar's leftover width. "lead" (default) lets the
   * breadcrumb side spread out and keeps the actions at their natural size,
   * which is what every titled desktop bar wants. "actions" hands the slack to
   * the trailing zone instead, for a bar whose action cluster is itself
   * flexible (the phone bar's scrolling strip): that cluster is `flex-1`, so
   * its base size is zero, and inside a zone that cannot grow it would resolve
   * to zero width no matter how much room the bar has.
   */
  freeWidth?: "lead" | "actions";
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
  titleSlot,
  subtitle,
  icon,
  homeAffordance,
  homeHref,
  crumbsCollapsed,
}: {
  backHref: string;
  backLabel: string;
  parents: ParentCrumb[] | undefined;
  title: string;
  titleSlot?: ReactNode;
  subtitle?: string;
  icon?: ReactNode;
  homeAffordance: "none" | "phone" | "always";
  homeHref: string;
  crumbsCollapsed: boolean;
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
    <Breadcrumb className="relative z-10 min-w-0 select-none" aria-label={t("common:breadcrumb")}>
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
        <ParentCrumbs parents={parents} forceCollapsed={crumbsCollapsed} />
        <BreadcrumbItem className="min-w-0">
          <TitleCrumb title={title} titleSlot={titleSlot} subtitle={subtitle} icon={icon} />
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}

const TITLE_CRUMB_CLASSNAME = "flex min-w-0 items-center gap-2";

/**
 * The current-page crumb.
 *
 * Plain text uses `BreadcrumbPage`, whose `role="link" aria-disabled="true"` is
 * the right shape for a dead-end label. A `titleSlot` is not that: it carries
 * its own interactive control (the inline rename), which must not be announced
 * as a disabled link, and flipping `aria-disabled` off would leave the wrapper
 * claiming a link role it never navigates. So a custom title gets plain
 * current-page markup instead, and the slot owns its own accessible name.
 */
function TitleCrumb({
  title,
  titleSlot,
  subtitle,
  icon,
}: {
  title: string;
  titleSlot?: ReactNode;
  subtitle?: string;
  icon?: ReactNode;
}) {
  const body = (
    <>
      {icon}
      {titleSlot ?? <span className="truncate text-sm font-medium">{title}</span>}
      {subtitle && (
        <>
          <span className="hidden text-muted-foreground/50 sm:inline">·</span>
          <span className="hidden truncate text-xs text-muted-foreground sm:inline">
            {subtitle}
          </span>
        </>
      )}
    </>
  );
  if (!titleSlot) return <BreadcrumbPage className={TITLE_CRUMB_CLASSNAME}>{body}</BreadcrumbPage>;
  return (
    <span
      data-slot="breadcrumb-page"
      aria-current="page"
      className={cn("font-normal text-foreground", TITLE_CRUMB_CLASSNAME)}
    >
      {body}
    </span>
  );
}

function ParentCrumbLabel({ crumb }: { crumb: ParentCrumb }) {
  if (crumb.href) {
    return (
      <>
        <BreadcrumbLink asChild>
          <Link
            href={crumb.href}
            className={cn(
              "max-w-40 truncate cursor-pointer text-muted-foreground underline-offset-4 transition-colors hover:text-foreground hover:underline",
              crumb.phoneOnlyLink && "md:hidden",
            )}
          >
            {crumb.label}
          </Link>
        </BreadcrumbLink>
        {crumb.phoneOnlyLink && (
          <span className="hidden max-w-40 cursor-default truncate text-muted-foreground/60 md:inline">
            {crumb.label}
          </span>
        )}
      </>
    );
  }
  // Static crumb: orientation only, dimmed so it does not read as a link. The
  // `title` is what makes a name longer than `max-w-40` readable at all, since
  // truncation is the only thing standing between a long label and a squeezed
  // page title.
  return (
    <span title={crumb.label} className="max-w-40 cursor-default truncate text-muted-foreground/60">
      {crumb.label}
    </span>
  );
}

/**
 * Middle crumbs: the full chain from `md` up; below `md` everything except the
 * last parent collapses into a `…` dropdown so deep paths stay one row —
 * `🏠 › … › Kanban1 › Integrations`. `forceCollapsed` applies the same
 * collapsed form at any width, when the space policy reports the full chain
 * would squeeze the title below its floor.
 */
function ParentCrumbs({
  parents,
  forceCollapsed,
}: {
  parents: ParentCrumb[] | undefined;
  forceCollapsed: boolean;
}) {
  const { t } = useTranslation();
  if (!parents || parents.length === 0) return null;
  const collapsed = parents.slice(0, -1);
  const lastParent = parents[parents.length - 1];
  const expandedClass = forceCollapsed ? "hidden shrink-0" : "max-md:hidden shrink-0";
  const collapsedClass = forceCollapsed ? "shrink-0" : "shrink-0 md:hidden";
  return (
    <>
      {collapsed.flatMap((p) => [
        <BreadcrumbItem key={`${p.href ?? p.label}-item`} className={expandedClass}>
          <ParentCrumbLabel crumb={p} />
        </BreadcrumbItem>,
        <BreadcrumbSeparator key={`${p.href ?? p.label}-sep`} className={expandedClass} />,
      ])}
      {collapsed.length > 0 && (
        <>
          <BreadcrumbItem className={collapsedClass}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  // `BreadcrumbEllipsis` is `aria-hidden`, which hides its own
                  // sr-only text too, so the trigger needs a name of its own.
                  // `-m-2 p-2` grows the touch target on this control without
                  // moving the crumbs around it.
                  aria-label={t("common:showMoreBreadcrumbs")}
                  className="-m-2 flex cursor-pointer items-center p-2 text-muted-foreground transition-colors hover:text-foreground"
                  data-testid="topbar-crumb-overflow"
                >
                  <BreadcrumbEllipsis />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                {collapsed.map((p) =>
                  p.href ? (
                    <DropdownMenuItem key={p.href} asChild>
                      <Link href={p.href}>{p.label}</Link>
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem key={p.label} disabled>
                      {p.label}
                    </DropdownMenuItem>
                  ),
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </BreadcrumbItem>
          <BreadcrumbSeparator className={collapsedClass} />
        </>
      )}
      <BreadcrumbItem key={`${lastParent.href ?? lastParent.label}-item`} className="shrink-0">
        <ParentCrumbLabel crumb={lastParent} />
      </BreadcrumbItem>
      <BreadcrumbSeparator className="shrink-0" />
    </>
  );
}

function GhostSeparator() {
  return <IconChevronRight className="size-3.5 shrink-0" />;
}

/**
 * Ghost text rendered as CSS generated content instead of a text node, so the
 * invisible measurement row can never satisfy a text locator or a screen
 * reader; only its width exists.
 */
function GhostLabel({ label, className }: { label: string; className?: string }) {
  return <span data-label={label} className={cn("before:content-[attr(data-label)]", className)} />;
}

type TopbarGhostProps = {
  ghostRef: RefObject<HTMLDivElement | null>;
  backHref: string;
  backLabel: string;
  homeAffordance: "none" | "phone" | "always";
  parents: ParentCrumb[] | undefined;
  title: string;
  subtitle?: string;
};

/**
 * Invisible measurement row for `useTopbarPressure`. Child order is the hook's
 * contract: [0] the expanded chain, [1] the collapsed chain, [2] the title
 * cluster. Mirrors the real crumbs' typography classes so widths transfer.
 */
function TopbarGhost({
  ghostRef,
  backHref,
  backLabel,
  homeAffordance,
  parents,
  title,
  subtitle,
}: TopbarGhostProps) {
  const showBack = backHref !== "/" && !!backLabel;
  const chain = parents ?? [];
  const lastParent = chain.length > 0 ? chain[chain.length - 1] : undefined;
  const homeLead =
    !showBack && homeAffordance !== "none" ? (
      <span
        className={cn("flex items-center gap-1.5", homeAffordance === "phone" && "md:hidden")}
        data-testid="topbar-ghost-home"
      >
        <IconHome className="h-4 w-4 shrink-0" />
        <GhostSeparator />
      </span>
    ) : null;
  const lead = showBack ? (
    <>
      <IconArrowLeft className="h-3.5 w-3.5 shrink-0" />
      <GhostLabel label={backLabel} />
      <GhostSeparator />
    </>
  ) : (
    homeLead
  );
  return (
    <div
      ref={ghostRef}
      aria-hidden
      className="pointer-events-none invisible absolute left-0 top-0 -z-10 flex items-center gap-3 whitespace-nowrap text-sm"
    >
      <span className="flex items-center gap-1.5">
        {lead}
        {chain.map((p, index) => (
          <span key={`${p.href ?? p.label}-${index}`} className="flex items-center gap-1.5">
            <GhostLabel label={p.label} className="max-w-40 truncate" />
            <GhostSeparator />
          </span>
        ))}
      </span>
      <span className="flex items-center gap-1.5">
        {lead}
        {chain.length > 1 && (
          <>
            <IconDots className="size-4 shrink-0" />
            <GhostSeparator />
          </>
        )}
        {lastParent && (
          <>
            <GhostLabel label={lastParent.label} className="max-w-40 truncate" />
            <GhostSeparator />
          </>
        )}
      </span>
      <span className="flex items-center gap-2">
        <GhostLabel label={title} className="font-medium" />
        {subtitle && <GhostLabel label={subtitle} className="text-xs" />}
      </span>
    </div>
  );
}

function TopbarCenter({
  center,
  centerClassName,
}: {
  center: ReactNode;
  centerClassName?: string;
}) {
  return (
    <>
      <div className={cn("relative z-10 flex shrink-0 items-center", centerClassName)}>
        {center}
      </div>
      {/* Balances the growing lead zone so the center floats mid-gap. */}
      <div aria-hidden className="grow" />
    </>
  );
}

type TopbarRightZoneProps = {
  zoneRef: RefObject<HTMLDivElement | null>;
  actions?: ReactNode;
  overflowActions?: ReactNode;
  actionsOverflowed: boolean;
  actionsClassName?: string;
  claimsFreeWidth: boolean;
  showStatusTrigger: boolean;
};

function TopbarRightZone({
  zoneRef,
  actions,
  overflowActions,
  actionsOverflowed,
  actionsClassName,
  claimsFreeWidth,
  showStatusTrigger,
}: TopbarRightZoneProps) {
  const hasCluster = Boolean(actions || overflowActions);
  if (!hasCluster && !showStatusTrigger) return null;
  return (
    // Default: no `min-w-0`, so the zone's automatic minimum is its min-content
    // width, a `shrink-0` cluster stays pinned at full width, and every shrink
    // goes to the lead zone.
    // `claimsFreeWidth`: the cluster is the flexible one, so this zone has to
    // grow — a `flex-1` cluster has a zero base size and would otherwise
    // resolve to zero width inside a zone that only ever shrinks. `min-w-0`
    // comes with it so the cluster can still scroll rather than overflow.
    <div
      ref={zoneRef}
      className={cn("flex items-center gap-3", claimsFreeWidth ? "min-w-0 grow" : "shrink")}
    >
      {hasCluster && (
        <div className={cn("relative z-10 flex shrink-0 items-center gap-2", actionsClassName)}>
          {!actionsOverflowed && overflowActions}
          {actions}
          {actionsOverflowed && overflowActions != null && (
            <TopbarActionsOverflow>{overflowActions}</TopbarActionsOverflow>
          )}
        </div>
      )}
      {showStatusTrigger ? (
        <AppStatusDrawerTrigger className={cn(hasCluster && "ml-1", "shrink-0")} />
      ) : null}
    </div>
  );
}

/** Trailing "…" menu the designated overflow actions fold into under pressure. */
function TopbarActionsOverflow({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={t("common:showMoreActions")}
          // `-m-2 p-2` grows the touch target without moving the actions
          // around it — same pattern as the crumb overflow trigger.
          className="-m-2 flex cursor-pointer items-center p-2 text-muted-foreground transition-colors hover:text-foreground"
          data-testid="topbar-actions-overflow"
        >
          <IconDots className="size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="flex flex-col gap-1 p-1">
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
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
    titleSlot,
    leading,
    center,
    leftActions,
    actions,
    overflowActions,
    className,
    freeWidth = "lead",
    centerClassName,
    actionsClassName,
    showStatusTrigger = true,
    homeAffordance = "phone",
    homeHref,
    testId,
  },
  ref,
) {
  const leadZoneRef = useRef<HTMLDivElement>(null);
  const ghostRef = useRef<HTMLDivElement>(null);
  const rightZoneRef = useRef<HTMLDivElement>(null);
  const actionsClaimFreeWidth = freeWidth === "actions";
  // Measurement only matters once there is something that can fold.
  const measured = (parents?.length ?? 0) > 0 || overflowActions != null;
  const pressure = useTopbarPressure(
    { leadZone: leadZoneRef, ghost: ghostRef, rightZone: rightZoneRef },
    measured,
  );
  return (
    <header
      ref={ref}
      data-testid={testId}
      data-window-controls-overlay-region="content"
      className={cn(
        "relative flex shrink-0 items-center gap-3 border-b px-3 py-1",
        TOPBAR_HEIGHT_CLASSNAME,
        className,
      )}
    >
      {measured && (
        <TopbarGhost
          ghostRef={ghostRef}
          backHref={backHref}
          backLabel={backLabel}
          homeAffordance={homeAffordance}
          parents={parents}
          title={title}
          subtitle={subtitle}
        />
      )}
      <div
        ref={leadZoneRef}
        className={cn(
          "flex min-w-0 items-center gap-3",
          // Only one zone can own the slack. When the actions take it, this one
          // sizes to its content instead of spreading across the empty middle.
          actionsClaimFreeWidth ? "shrink" : "grow",
        )}
      >
        {leading}
        <TopbarBreadcrumb
          backHref={backHref}
          backLabel={backLabel}
          parents={parents}
          title={title}
          titleSlot={titleSlot}
          subtitle={subtitle}
          icon={icon}
          homeAffordance={homeAffordance}
          homeHref={homeHref ?? linkToTaskOverview()}
          crumbsCollapsed={pressure.crumbsCollapsed}
        />
        {leftActions && (
          <div className="relative z-10 flex shrink-0 items-center gap-1 [&:empty]:hidden">
            {leftActions}
          </div>
        )}
      </div>
      {center && <TopbarCenter center={center} centerClassName={centerClassName} />}
      <TopbarRightZone
        zoneRef={rightZoneRef}
        actions={actions}
        overflowActions={overflowActions}
        actionsOverflowed={pressure.actionsOverflowed}
        actionsClassName={actionsClassName}
        claimsFreeWidth={actionsClaimFreeWidth}
        showStatusTrigger={showStatusTrigger}
      />
    </header>
  );
});
