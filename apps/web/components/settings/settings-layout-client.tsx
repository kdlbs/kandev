"use client";

import { usePathname } from "next/navigation";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@kandev/ui/sidebar";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageTopbar } from "@/components/page-topbar";
import { SettingsAppSidebar } from "@/components/settings/settings-app-sidebar";

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

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";

  if (isAgentDetail) {
    return (
      <SettingsShell
        title="Agent"
        backHref="/settings/agents"
        backLabel="Agents"
        parent={undefined}
      >
        {children}
      </SettingsShell>
    );
  }

  const pageLabel = deriveCurrentPageLabel(pathname);
  const title = pageLabel ?? "Settings";
  const parent = pageLabel ? { label: "Settings", href: "/settings" } : undefined;

  return (
    <SettingsShell title={title} backHref="/" backLabel="Kandev" parent={parent}>
      {children}
    </SettingsShell>
  );
}

function SettingsShell({
  title,
  backHref,
  backLabel,
  parent,
  children,
}: {
  title: string;
  backHref: string;
  backLabel: string;
  parent: { label: string; href: string } | undefined;
  children: React.ReactNode;
}) {
  return (
    <TooltipProvider>
      <SidebarProvider>
        <SettingsAppSidebar />
        <SidebarInset>
          <PageTopbar
            title={title}
            backHref={backHref}
            backLabel={backLabel}
            parent={parent}
            className="h-16 border-b-0"
            leading={<SidebarTrigger size="lg" className="md:hidden h-10 w-10 cursor-pointer" />}
          />
          <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pt-0 mb-20">{children}</div>
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
