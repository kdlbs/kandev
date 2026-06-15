"use client";

import { usePathname } from "next/navigation";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageTopbar } from "@/components/page-topbar";

// Brand/initialism overrides so the derived label matches how the rest of the
// app spells these (e.g. "github" → "GitHub", not "Github"). Anything not
// listed here falls back to dash-aware title-casing of the path segment.
const SEGMENT_LABEL_OVERRIDES: Record<string, string> = {
  github: "GitHub",
  jira: "Jira",
  linear: "Linear",
  slack: "Slack",
  mcp: "MCP",
  ui: "UI",
  vscode: "VS Code",
};

function titleCase(segment: string): string {
  if (SEGMENT_LABEL_OVERRIDES[segment]) return SEGMENT_LABEL_OVERRIDES[segment];
  return segment
    .split("-")
    .map((p) => (p.length === 0 ? p : p[0].toUpperCase() + p.slice(1)))
    .join(" ");
}

// Derive the human-readable label for the current /settings sub-page from the
// deepest non-id path segment. /settings → null (the topbar still shows
// "Settings" as the page itself). UUID-looking segments are skipped so e.g.
// /settings/workspace/<uuid> resolves to "Workspace" not the raw id.
function deriveCurrentPageLabel(pathname: string): string | null {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return null; // just /settings
  for (let i = segments.length - 1; i >= 1; i--) {
    const seg = segments[i];
    if (/^[0-9a-f-]{8,}$/i.test(seg)) continue; // skip ids
    return titleCase(seg);
  }
  return null;
}

// Build the intermediate breadcrumb crumbs between the back link and the
// current page title. For workspace-scoped automation pages, inject an
// "Automations" crumb so the breadcrumb reads e.g.
// Home > Settings > Automations > New.
function deriveParents(pathname: string): Array<{ label: string; href: string }> {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return [];

  const parents: Array<{ label: string; href: string }> = [
    { label: "Settings", href: "/settings" },
  ];

  const automationsMatch = pathname.match(
    /^\/settings\/workspace\/([^/]+)\/automations(?:\/(.+))?/,
  );
  if (automationsMatch && automationsMatch[2]) {
    // Only inject the Automations crumb when we're on a sub-page (new or
    // edit), not on the listing page itself — the listing page title is
    // already "Automations".
    parents.push({
      label: "Automations",
      href: `/settings/workspace/${automationsMatch[1]}/automations`,
    });
  }

  return parents;
}

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";

  if (isAgentDetail) {
    return (
      <SettingsShell title="Agent" backHref="/settings/agents" backLabel="Agents" parents={[]}>
        {children}
      </SettingsShell>
    );
  }

  const pageLabel = deriveCurrentPageLabel(pathname);
  const title = pageLabel ?? "Settings";
  const parents = deriveParents(pathname);

  return (
    <SettingsShell title={title} backHref="/" backLabel="Kandev" parents={parents}>
      {children}
    </SettingsShell>
  );
}

function SettingsShell({
  title,
  backHref,
  backLabel,
  parents,
  children,
}: {
  title: string;
  backHref: string;
  backLabel: string;
  parents: Array<{ label: string; href: string }>;
  children: React.ReactNode;
}) {
  return (
    <TooltipProvider>
      <main className="flex min-h-0 flex-1 flex-col">
        <PageTopbar
          title={title}
          backHref={backHref}
          backLabel={backLabel}
          parents={parents}
          className="h-10"
        />
        {/* Scroll the content, not the topbar: min-h-0 lets this flex child
            shrink below its content height so overflow-y-auto can take effect. */}
        <div
          data-testid="settings-scroll-container"
          className="flex min-w-0 min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain p-4 pb-20"
        >
          {children}
        </div>
      </main>
    </TooltipProvider>
  );
}
