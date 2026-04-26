"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  IconSquarePlus,
  IconLayoutDashboard,
  IconInbox,
  IconCircleDot,
  IconRepeat,
  IconNetwork,
  IconBoxMultiple,
  IconCurrencyDollar,
  IconHistory,
  IconSettings,
  IconSearch,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { WorkspaceSwitcher } from "@/components/task/workspace-switcher";
import { useAppStore } from "@/components/state-provider";
import { SidebarNavItem } from "./sidebar-nav-item";
import { SidebarSection } from "./sidebar-section";
import { SidebarAgentsList } from "./sidebar-agents-list";
import { SidebarProjectsList } from "./sidebar-projects-list";
import { NewIssueDialog } from "./new-issue-dialog";

export function OrchestrateSidebar() {
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const inboxCount = useAppStore((s) => s.orchestrate.inboxCount);
  const [newIssueOpen, setNewIssueOpen] = useState(false);

  const handleWorkspaceSelect = (workspaceId: string) => {
    setActiveWorkspace(workspaceId);
    router.push(`/orchestrate?workspaceId=${workspaceId}`);
  };

  return (
    <aside className="w-60 h-full min-h-0 border-r border-border bg-background flex flex-col">
      {/* Top: workspace switcher + search */}
      <div className="flex items-center gap-1 px-3 h-12 border-b border-border">
        <div className="flex-1 min-w-0">
          <WorkspaceSwitcher
            workspaces={workspaces.items}
            activeWorkspaceId={workspaces.activeId}
            onSelect={handleWorkspaceSelect}
          />
        </div>
        <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0 cursor-pointer">
          <IconSearch className="h-4 w-4" />
        </Button>
      </div>

      {/* Nav: scrollable */}
      <nav className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-3 py-2">
        {/* Top actions */}
        <div className="flex flex-col gap-0.5">
          <SidebarNavItem icon={IconSquarePlus} label="New Issue" href="/orchestrate/issues" onClick={() => setNewIssueOpen(true)} />
          <SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/orchestrate" />
          <SidebarNavItem icon={IconInbox} label="Inbox" href="/orchestrate/inbox" badge={inboxCount} />
        </div>

        {/* Work section */}
        <SidebarSection label="Work">
          <SidebarNavItem icon={IconCircleDot} label="Issues" href="/orchestrate/issues" />
          <SidebarNavItem icon={IconRepeat} label="Routines" href="/orchestrate/routines" />
        </SidebarSection>

        {/* Projects section (collapsible) */}
        <SidebarProjectsList />

        {/* Agents section (collapsible) */}
        <SidebarAgentsList />

        {/* Company section */}
        <SidebarSection label="Company">
          <SidebarNavItem icon={IconNetwork} label="Org" href="/orchestrate/company/org" />
          <SidebarNavItem icon={IconBoxMultiple} label="Skills" href="/orchestrate/company/skills" />
          <SidebarNavItem icon={IconCurrencyDollar} label="Costs" href="/orchestrate/company/costs" />
          <SidebarNavItem icon={IconHistory} label="Activity" href="/orchestrate/company/activity" />
          <SidebarNavItem icon={IconSettings} label="Settings" href="/orchestrate/company/settings" />
        </SidebarSection>
      </nav>

      <NewIssueDialog open={newIssueOpen} onOpenChange={setNewIssueOpen} />
    </aside>
  );
}
