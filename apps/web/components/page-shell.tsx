"use client";

import type { ReactNode } from "react";
import { AppNavSheet } from "@/components/navigation/app-nav-sheet";
import { PageTopbar } from "@/components/page-topbar";
import { useHomeAffordance } from "@/hooks/use-home-affordance";
import { cn } from "@/lib/utils";

type PageShellProps = {
  // Forwarded verbatim to PageTopbar.
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  backHref?: string;
  backLabel?: string;
  parents?: Array<{ label: string; href: string }>;
  variant?: "breadcrumb" | "root";
  center?: ReactNode;
  leading?: ReactNode;
  leftActions?: ReactNode;
  actions?: ReactNode;
  className?: string;
  showStatusTrigger?: boolean;
  /** `data-testid` on the topbar header (e2e anchors like `office-topbar`). */
  topbarTestId?: string;
  /** In-section nav for the phone sheet: settings tree, office sections. */
  pageNav?: ReactNode;
  /** Manifest destination ids `pageNav` already covers. See `AppNavSections`. */
  navOmitDestinations?: string[];
  /** "none" for children that own their own scrolling (dockview, full-bleed). */
  scroll?: "auto" | "none";
  contentClassName?: string;
  /** `data-testid` on the scroll container (e2e anchors like `settings-scroll-container`). */
  contentTestId?: string;
  children: ReactNode;
};

/**
 * The one page chrome for top-level routes: `PageTopbar` with the app nav
 * trigger injected ahead of any page `leading`, the home affordance resolved by
 * `useHomeAffordance` (instead of each shell hand-rolling an escape hatch), and
 * a scroll container below.
 *
 * Renders no `<main>` — `AppShell` already owns that landmark; Settings used to
 * nest a second one.
 */
export function PageShell({
  variant = "breadcrumb",
  leading,
  pageNav,
  navOmitDestinations,
  scroll = "auto",
  contentClassName,
  contentTestId,
  children,
  ...topbarProps
}: PageShellProps) {
  const home = useHomeAffordance(variant);
  const { topbarTestId, ...forwarded } = topbarProps;
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <PageTopbar
        {...forwarded}
        variant={variant}
        testId={topbarTestId}
        homeAffordance={home.mode}
        homeHref={home.href}
        leading={
          <>
            <AppNavSheet pageNav={pageNav} omitDestinations={navOmitDestinations} />
            {leading}
          </>
        }
      />
      {scroll === "none" ? (
        children
      ) : (
        <div
          data-testid={contentTestId}
          className={cn("min-h-0 min-w-0 flex-1 overflow-y-auto", contentClassName)}
        >
          {children}
        </div>
      )}
    </div>
  );
}
