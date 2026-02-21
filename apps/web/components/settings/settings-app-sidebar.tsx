"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  IconSettings,
  IconFolder,
  IconServer,
  IconRobot,
  IconBell,
  IconCode,
  IconCpu,
  IconKey,
  IconSparkles,
  IconMessageCircle,
} from "@tabler/icons-react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarMenuSub,
  SidebarMenuSubItem,
  SidebarMenuSubButton,
  SidebarHeader,
  useSidebar,
} from "@kandev/ui/sidebar";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AgentLogo } from "@/components/agent-logo";
import type { Workspace, Agent, AgentProfile, Environment, Executor } from "@/lib/types/http";

type GeneralSidebarSectionProps = {
  pathname: string;
};

function GeneralSidebarSection({ pathname }: GeneralSidebarSectionProps) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild tooltip="General">
        <Link href="/settings/general">
          <IconSettings className="h-4 w-4" />
          <span>General</span>
        </Link>
      </SidebarMenuButton>
      <SidebarMenuSub className="ml-3 mt-1">
        <SidebarMenuSubItem>
          <SidebarMenuSubButton
            asChild
            size="sm"
            isActive={pathname === "/settings/general/notifications"}
          >
            <Link href="/settings/general/notifications">
              <IconBell className="h-4 w-4" />
              <span>Notifications</span>
            </Link>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
        <SidebarMenuSubItem>
          <SidebarMenuSubButton
            asChild
            size="sm"
            isActive={pathname === "/settings/general/editors"}
          >
            <Link href="/settings/general/editors">
              <IconCode className="h-4 w-4" />
              <span>Editors</span>
            </Link>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
        <SidebarMenuSubItem>
          <SidebarMenuSubButton
            asChild
            size="sm"
            isActive={pathname === "/settings/general/secrets"}
          >
            <Link href="/settings/general/secrets">
              <IconKey className="h-4 w-4" />
              <span>Secrets</span>
            </Link>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
        <SidebarMenuSubItem>
          <SidebarMenuSubButton
            asChild
            size="sm"
            isActive={pathname === "/settings/general/sprites"}
          >
            <Link href="/settings/general/sprites">
              <IconSparkles className="h-4 w-4" />
              <span>Sprites.dev</span>
            </Link>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
      </SidebarMenuSub>
    </SidebarMenuItem>
  );
}

type WorkspacesSidebarSectionProps = {
  pathname: string;
  workspaces: Workspace[];
};

function WorkspacesSidebarSection({ pathname, workspaces }: WorkspacesSidebarSectionProps) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild tooltip="Workspaces">
        <Link href="/settings/workspace">
          <IconFolder className="h-4 w-4" />
          <span>Workspaces</span>
        </Link>
      </SidebarMenuButton>
      {workspaces.length > 0 && (
        <SidebarMenuSub className="ml-3 mt-1">
          {workspaces.map((workspace: Workspace) => {
            const workspacePath = `/settings/workspace/${workspace.id}`;
            const workflowsPath = `${workspacePath}/workflows`;
            const repositoriesPath = `${workspacePath}/repositories`;

            return (
              <SidebarMenuSubItem key={workspace.id}>
                <SidebarMenuSubButton asChild isActive={pathname === workspacePath}>
                  <Link href={workspacePath}>
                    <span>{workspace.name}</span>
                  </Link>
                </SidebarMenuSubButton>
                <SidebarMenuSub className="ml-3">
                  <SidebarMenuSubItem>
                    <SidebarMenuSubButton
                      asChild
                      size="sm"
                      isActive={pathname === repositoriesPath}
                    >
                      <Link href={repositoriesPath}>
                        <span>Repositories</span>
                      </Link>
                    </SidebarMenuSubButton>
                  </SidebarMenuSubItem>
                  <SidebarMenuSubItem>
                    <SidebarMenuSubButton asChild size="sm" isActive={pathname === workflowsPath}>
                      <Link href={workflowsPath}>
                        <span>Workflows</span>
                      </Link>
                    </SidebarMenuSubButton>
                  </SidebarMenuSubItem>
                </SidebarMenuSub>
              </SidebarMenuSubItem>
            );
          })}
        </SidebarMenuSub>
      )}
    </SidebarMenuItem>
  );
}

type AgentsSidebarSectionProps = {
  pathname: string;
  agents: Agent[];
};

function AgentsSidebarSection({ pathname, agents }: AgentsSidebarSectionProps) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild tooltip="Agents">
        <Link href="/settings/agents">
          <IconRobot className="h-4 w-4" />
          <span>Agents</span>
        </Link>
      </SidebarMenuButton>
      {agents.length > 0 && (
        <SidebarMenuSub className="ml-3 mt-1">
          {agents.flatMap((agent: Agent) =>
            agent.profiles.map((profile: AgentProfile) => {
              const encodedAgent = encodeURIComponent(agent.name);
              const profilePath = `/settings/agents/${encodedAgent}/profiles/${profile.id}`;
              const agentLabel = profile.agent_display_name || agent.name;
              return (
                <SidebarMenuSubItem key={profile.id} className="min-w-0">
                  <SidebarMenuSubButton asChild isActive={pathname === profilePath}>
                    <Link
                      href={profilePath}
                      className="!flex min-w-0 items-center gap-1.5"
                      title={`${agentLabel} • ${profile.name}`}
                    >
                      <AgentLogo agentName={agent.name} className="shrink-0" />
                      <ScrollOnOverflow className="min-w-0">
                        {agentLabel} • {profile.name}
                      </ScrollOnOverflow>
                    </Link>
                  </SidebarMenuSubButton>
                </SidebarMenuSubItem>
              );
            }),
          )}
        </SidebarMenuSub>
      )}
    </SidebarMenuItem>
  );
}

type ResourceSidebarSectionProps = {
  pathname: string;
  executors: Executor[];
  environments: Environment[];
};

function ResourceSidebarSection({
  pathname,
  executors,
  environments,
}: ResourceSidebarSectionProps) {
  return (
    <>
      {/* Executors */}
      <SidebarMenuItem>
        <SidebarMenuButton asChild tooltip="Executors">
          <Link href="/settings/executors">
            <IconCpu className="h-4 w-4" />
            <span>Executors</span>
          </Link>
        </SidebarMenuButton>
        {executors.length > 0 && (
          <SidebarMenuSub className="ml-3 mt-1">
            {executors.map((executor: Executor) => (
              <SidebarMenuSubItem key={executor.id}>
                <SidebarMenuSubButton
                  asChild
                  isActive={pathname === `/settings/executor/${executor.id}`}
                >
                  <Link href={`/settings/executor/${executor.id}`}>
                    <span>{executor.name}</span>
                  </Link>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            ))}
          </SidebarMenuSub>
        )}
      </SidebarMenuItem>

      {/* Environments */}
      <SidebarMenuItem>
        <SidebarMenuButton asChild tooltip="Environments">
          <Link href="/settings/environments">
            <IconServer className="h-4 w-4" />
            <span>Environments</span>
          </Link>
        </SidebarMenuButton>
        {environments.length > 0 && (
          <SidebarMenuSub className="ml-3 mt-1">
            {environments.map((env: Environment) => (
              <SidebarMenuSubItem key={env.id}>
                <SidebarMenuSubButton
                  asChild
                  isActive={pathname === `/settings/environment/${env.id}`}
                >
                  <Link href={`/settings/environment/${env.id}`}>
                    <span>{env.name}</span>
                  </Link>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            ))}
          </SidebarMenuSub>
        )}
      </SidebarMenuItem>
    </>
  );
}

export function SettingsAppSidebar() {
  const pathname = usePathname();
  const { setOpenMobile, isMobile } = useSidebar();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const environments = useAppStore((state) => state.environments.items);
  const executors = useAppStore((state) => state.executors.items);
  const agents = useAppStore((state) => state.settingsAgents.items);
  useAvailableAgents();

  // Close mobile sidebar when navigating to a new page
  useEffect(() => {
    if (isMobile) {
      setOpenMobile(false);
    }
  }, [pathname, isMobile, setOpenMobile]);

  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href="/">
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">Kandev</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent className="overflow-hidden">
        <ScrollArea
          className="h-full [&_[data-slot=scroll-area-viewport]>div]:!block [&_[data-slot=scroll-area-viewport]>div]:!min-w-0"
          type="always"
        >
          <SidebarGroup>
            <SidebarGroupLabel>Settings</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <GeneralSidebarSection pathname={pathname} />
                <WorkspacesSidebarSection pathname={pathname} workspaces={workspaces} />
                <AgentsSidebarSection pathname={pathname} agents={agents} />

                {/* Prompts */}
                <SidebarMenuItem>
                  <SidebarMenuButton asChild isActive={pathname === "/settings/prompts"}>
                    <Link href="/settings/prompts">
                      <IconMessageCircle className="h-4 w-4" />
                      <span>Prompts</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>

                <ResourceSidebarSection
                  pathname={pathname}
                  executors={executors}
                  environments={environments}
                />
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </ScrollArea>
      </SidebarContent>
    </Sidebar>
  );
}
