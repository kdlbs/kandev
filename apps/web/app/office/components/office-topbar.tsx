"use client";

import { usePathname } from "next/navigation";

const PAGE_TITLES: Record<string, string> = {
  "/office": "Dashboard",
  "/office/inbox": "Inbox",
  "/office/tasks": "Tasks",
  "/office/routines": "Routines",
  "/office/projects": "Projects",
  "/office/agents": "Agents",
  "/office/workspace/org": "Org Chart",
  "/office/workspace/skills": "Skills",
  "/office/workspace/costs": "Costs",
  "/office/workspace/activity": "Activity",
  "/office/workspace/routing": "Provider Routing",
  "/office/workspace/settings": "Settings",
};

function resolveTitle(pathname: string): string | null {
  const exact = PAGE_TITLES[pathname];
  if (exact) return exact;
  if (pathname.startsWith("/office/workspace/settings")) return "Settings";
  return null;
}

function isDetailPage(pathname: string): boolean {
  return (
    /^\/office\/tasks\/[^/]+$/.test(pathname) ||
    /^\/office\/agents\/[^/]+$/.test(pathname) ||
    /^\/office\/projects\/[^/]+$/.test(pathname) ||
    /^\/office\/routines\/[^/]+$/.test(pathname)
  );
}

/**
 * Office topbar. For list pages shows a static title. For detail pages
 * renders a portal target (#office-topbar-slot) that the page component
 * fills with its breadcrumb via OfficeTopbarPortal.
 */
export function OfficeTopbar() {
  const pathname = usePathname();
  const title = resolveTitle(pathname);
  const detail = isDetailPage(pathname);

  return (
    <div className="flex items-center gap-2 px-4 h-12 border-b border-border bg-background shrink-0">
      {detail ? (
        <div id="office-topbar-slot" className="flex items-center gap-2 flex-1 min-w-0" />
      ) : (
        title && (
          <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            {title}
          </h1>
        )
      )}
    </div>
  );
}
