"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconBoxMultiple, IconInbox, IconRobot, IconSitemap } from "@tabler/icons-react";
import { AppSidebarNavItem } from "@/components/app-sidebar/app-sidebar-nav-item";
import { OfficeNavigationSection } from "@/components/app-sidebar/sections/office-navigation-section";
import { PageShell } from "@/components/page-shell";
import { usePathname } from "@/lib/routing/client-router";
// Route -> catalog key, not route -> title. The map is module scope, so a `t()`
// here would resolve once at import and freeze at the boot locale; the keys are
// resolved at render below. The route paths are URLs, not copy.
const PAGE_TITLE_KEYS: Record<string, string> = {
  "/office": "office:dashboard",
  "/office/inbox": "office:inbox",
  "/office/tasks": "office:tasks",
  "/office/routines": "office:routines",
  "/office/projects": "office:projects",
  "/office/agents": "office:agents",
  "/office/workspace/org": "office:orgChart",
  "/office/workspace/skills": "office:skills",
  "/office/workspace/costs": "office:costs",
  "/office/workspace/activity": "office:activity",
  "/office/workspace/routing": "office:providerRouting",
  "/office/workspace/settings": "office:preferences",
};

function resolveTitleKey(pathname: string): string | null {
  const exact = PAGE_TITLE_KEYS[pathname];
  if (exact) return exact;
  if (pathname.startsWith("/office/workspace/settings")) return "office:preferences";
  return null;
}

function isDetailPage(pathname: string): boolean {
  return (
    /^\/office\/tasks\/[^/]+$/.test(pathname) ||
    /^\/office\/agents\/[^/]+(?:\/.*)?$/.test(pathname) ||
    /^\/office\/projects\/[^/]+$/.test(pathname) ||
    /^\/office\/routines\/[^/]+$/.test(pathname)
  );
}

// Destinations the desktop sidebar reaches through sections other than
// `OfficeNavigationSection` — Inbox via `AppSidebarPrimaryNav`, Projects via
// `ProjectsSection`, Agents and Org Chart via `AgentsSection`. Those three build
// rows from live office data and drive some of them through `router.push`
// buttons, which the sheet's close-on-link-click would miss, so the sheet
// restates the destinations as plain links instead of rendering the sections.
// Mobile owes desktop capability parity, not layout parity.
const OFFICE_SHEET_DESTINATIONS = [
  { icon: IconInbox, labelKey: "sidebar:inbox", href: "/office/inbox" },
  { icon: IconBoxMultiple, labelKey: "sidebar:projects", href: "/office/projects" },
  { icon: IconRobot, labelKey: "common:agents", href: "/office/agents" },
  { icon: IconSitemap, labelKey: "sidebar:orgChart", href: "/office/workspace/org" },
] as const;

/**
 * Office's sections for the phone nav sheet — the same component the desktop
 * sidebar renders, compacted to the sheet's touch-row sizing, plus the
 * destinations that live in the sidebar's other sections. Without this, office
 * navigation exists only in the `hidden md:block` sidebar.
 */
function OfficePageNav() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 [&_a]:min-h-10 [&_a]:text-sm [&_svg]:h-4 [&_svg]:w-4">
      <OfficeNavigationSection collapsed={false} section="work" />
      <div className="flex flex-col gap-0.5">
        {OFFICE_SHEET_DESTINATIONS.map(({ icon, labelKey, href }) => (
          <AppSidebarNavItem
            key={href}
            icon={icon}
            label={t(labelKey)}
            href={href}
            collapsed={false}
          />
        ))}
      </div>
      <OfficeNavigationSection collapsed={false} section="office" />
    </div>
  );
}

/**
 * Office page chrome on the shared `PageShell`. List pages show a static
 * title; detail pages render a portal target (#office-topbar-slot) that the
 * page component fills with its breadcrumb via OfficeTopbarPortal. Both paint
 * paths (`src/office-routes.tsx` and `app/office/layout.tsx`) wrap their route
 * output in this shell so they cannot drift.
 */
export function OfficeShell({
  children,
  routePath,
}: {
  children: ReactNode;
  /** Resolved route for the `data-office-route` anchor; defaults to the URL. */
  routePath?: string;
}) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const titleKey = resolveTitleKey(pathname);
  const detail = isDetailPage(pathname);
  const title = titleKey ? t(titleKey) : "";

  return (
    <PageShell
      title={title}
      variant="root"
      backLabel=""
      topbarTestId="office-topbar"
      className="gap-2 bg-background px-4"
      pageNav={<OfficePageNav />}
      scroll="none"
      // The manifest's Tasks row goes to kanban. Office's own Tasks row above it
      // goes to /office/tasks, and two rows labelled "Tasks" leading to
      // different surfaces is a coin flip for the user.
      navOmitDestinations={["tasks"]}
      leading={
        detail ? (
          <div id="office-topbar-slot" className="flex items-center gap-2 flex-1 min-w-0" />
        ) : (
          titleKey && <h1 className="truncate text-sm font-medium text-foreground">{title}</h1>
        )
      }
    >
      {/* `data-office-route` stamps the RESOLVED route onto the outlet, and is
          the render anchor the pseudo-coverage oracle waits on for every
          `office — …` screen (e2e/tests/i18n/pseudo-coverage.spec.ts).

          It is an attribute on the outlet rather than a testid inside each
          page because these pages are mounted with empty collections
          (`initialItems={[]}`), so they legitimately render empty states in
          e2e — an anchor inside the populated branch would never match. This
          attribute is present whichever branch a page takes, and it is not
          shell-satisfiable: this element lives in the office chunk, is not
          rendered while `OfficeRouteLoading` holds, and its VALUE identifies
          the specific route, so a mis-pointed URL cannot match another
          screen's anchor.

          The shell owns this outlet (`scroll="none"`) so both paint paths —
          `src/office-routes.tsx` and `app/office/layout.tsx` — carry the
          anchor without either one re-declaring it. */}
      <main className="flex-1 min-h-0 overflow-y-auto" data-office-route={routePath ?? pathname}>
        {children}
      </main>
    </PageShell>
  );
}
