"use client";

import { useState } from "react";
import {
  IconSquarePlus,
  IconLayoutDashboard,
  IconInbox,
  IconCircleDot,
  IconRepeat,
  IconSitemap,
  IconBoxMultiple,
  IconCurrencyDollar,
  IconHistory,
  IconSettings,
  IconSearch,
} from "@tabler/icons-react";
import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ThemeToggle } from "@/components/theme-toggle";
import { useAppStore } from "@/components/state-provider";
import { SidebarNavItem } from "./sidebar-nav-item";
import { SidebarSection } from "./sidebar-section";
import { SidebarAgentsList } from "./sidebar-agents-list";
import { SidebarProjectsList } from "./sidebar-projects-list";
import { NewIssueDialog } from "./new-issue-dialog";

interface OrchestrateSidebarProps {
  workspaceName?: string;
}

export function OrchestrateSidebar({ workspaceName: ssrName }: OrchestrateSidebarProps) {
  const workspaces = useAppStore((s) => s.workspaces);
  const inboxCount = useAppStore((s) => s.orchestrate.inboxCount);
  const [newIssueOpen, setNewIssueOpen] = useState(false);

  // Use store if hydrated, fall back to SSR prop
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const workspaceName = activeWorkspace?.name || ssrName || "Workspace";

  return (
    <aside className="w-60 h-full min-h-0 border-r border-border bg-background flex flex-col">
      {/* Top: workspace name + search */}
      <div className="flex items-center gap-1 px-3 h-12 border-b border-border">
        <span className="flex-1 min-w-0 text-sm font-bold truncate">{workspaceName}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0 cursor-pointer">
              <IconSearch className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Search</TooltipContent>
        </Tooltip>
      </div>

      {/* Nav: scrollable */}
      <nav className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-3 py-2">
        {/* Top actions */}
        <div className="flex flex-col gap-0.5">
          <SidebarNavItem icon={IconSquarePlus} label="New Task" href="/orchestrate/issues" onClick={() => setNewIssueOpen(true)} />
          <SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/orchestrate" />
          <SidebarNavItem icon={IconInbox} label="Inbox" href="/orchestrate/inbox" badge={inboxCount} />
        </div>

        {/* Work section */}
        <SidebarSection label="Work">
          <SidebarNavItem icon={IconCircleDot} label="Tasks" href="/orchestrate/issues" />
          <SidebarNavItem icon={IconRepeat} label="Routines" href="/orchestrate/routines" />
        </SidebarSection>

        {/* Projects section (collapsible) */}
        <SidebarProjectsList />

        {/* Agents section (collapsible) */}
        <SidebarAgentsList />

        {/* Company section */}
        <SidebarSection label="Workspace">
          <SidebarNavItem icon={IconSitemap} label="Org" href="/orchestrate/workspace/org" />
          <SidebarNavItem icon={IconBoxMultiple} label="Skills" href="/orchestrate/workspace/skills" />
          <SidebarNavItem icon={IconCurrencyDollar} label="Costs" href="/orchestrate/workspace/costs" />
          <SidebarNavItem icon={IconHistory} label="Activity" href="/orchestrate/workspace/activity" />
          <SidebarNavItem icon={IconSettings} label="Settings" href="/orchestrate/workspace/settings" />
        </SidebarSection>
      </nav>

      {/* Bottom bar */}
      <div className="flex items-center justify-end gap-1 px-3 h-10 border-t border-border shrink-0">
        <Tooltip>
          <TooltipTrigger asChild>
            <Link href="/settings/general" className="cursor-pointer">
              <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
                <IconSettings className="h-3.5 w-3.5" />
              </Button>
            </Link>
          </TooltipTrigger>
          <TooltipContent>Settings</TooltipContent>
        </Tooltip>
        <ThemeToggle />
      </div>

      <NewIssueDialog open={newIssueOpen} onOpenChange={setNewIssueOpen} />
    </aside>
  );
}
