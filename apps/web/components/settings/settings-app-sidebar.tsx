'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  IconSettings,
  IconFolder,
  IconServer,
  IconRobot,
  IconBell,
  IconCode,
  IconChevronRight,
  IconCpu,
  IconPlug,
  IconPuzzle,
} from '@tabler/icons-react';
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
  SidebarMenuAction,
  SidebarHeader,
} from '@kandev/ui/sidebar';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@kandev/ui/collapsible';
import { useAppStore } from '@/components/state-provider';

export function SettingsAppSidebar() {
  const pathname = usePathname();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const environments = useAppStore((state) => state.environments.items);
  const executors = useAppStore((state) => state.executors.items);
  const agents = useAppStore((state) => state.settingsAgents.items);

  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href="/">
                <div className="bg-primary text-primary-foreground flex aspect-square size-8 items-center justify-center rounded-lg font-bold text-sm">
                  K
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">KanDev.ai</span>
                  <span className="truncate text-xs">Settings</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Settings</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {/* General */}
              <Collapsible defaultOpen className="group/collapsible">
                <SidebarMenuItem>
                  <SidebarMenuButton asChild tooltip="General">
                    <Link href="/settings/general">
                      <IconSettings className="h-4 w-4" />
                      <span>General</span>
                    </Link>
                  </SidebarMenuButton>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuAction className="data-[state=open]:rotate-90">
                      <IconChevronRight className="h-4 w-4" />
                      <span className="sr-only">Toggle</span>
                    </SidebarMenuAction>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <SidebarMenuSub>
                      <SidebarMenuSubItem>
                        <SidebarMenuSubButton
                          asChild
                          size="sm"
                          isActive={pathname === '/settings/general/notifications'}
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
                          isActive={pathname === '/settings/general/editors'}
                        >
                          <Link href="/settings/general/editors">
                            <IconCode className="h-4 w-4" />
                            <span>Editors</span>
                          </Link>
                        </SidebarMenuSubButton>
                      </SidebarMenuSubItem>
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>

              {/* Workspaces */}
              <Collapsible defaultOpen className="group/collapsible">
                <SidebarMenuItem>
                  <SidebarMenuButton asChild tooltip="Workspaces">
                    <Link href="/settings/workspace">
                      <IconFolder className="h-4 w-4" />
                      <span>Workspaces</span>
                    </Link>
                  </SidebarMenuButton>
                  {workspaces.length > 0 && (
                    <>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuAction className="data-[state=open]:rotate-90">
                          <IconChevronRight className="h-4 w-4" />
                          <span className="sr-only">Toggle</span>
                        </SidebarMenuAction>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {workspaces.map((workspace) => {
                            const workspacePath = `/settings/workspace/${workspace.id}`;
                            const boardsPath = `${workspacePath}/boards`;
                            const repositoriesPath = `${workspacePath}/repositories`;

                            return (
                              <SidebarMenuSubItem key={workspace.id}>
                                <SidebarMenuSubButton
                                  asChild
                                  isActive={pathname === workspacePath}
                                >
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
                                    <SidebarMenuSubButton
                                      asChild
                                      size="sm"
                                      isActive={pathname === boardsPath}
                                    >
                                      <Link href={boardsPath}>
                                        <span>Boards</span>
                                      </Link>
                                    </SidebarMenuSubButton>
                                  </SidebarMenuSubItem>
                                </SidebarMenuSub>
                              </SidebarMenuSubItem>
                            );
                          })}
                        </SidebarMenuSub>
                      </CollapsibleContent>
                    </>
                  )}
                </SidebarMenuItem>
              </Collapsible>

              {/* Environments */}
              <Collapsible defaultOpen className="group/collapsible">
                <SidebarMenuItem>
                  <SidebarMenuButton asChild tooltip="Environments">
                    <Link href="/settings/environments">
                      <IconServer className="h-4 w-4" />
                      <span>Environments</span>
                    </Link>
                  </SidebarMenuButton>
                  {environments.length > 0 && (
                    <>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuAction className="data-[state=open]:rotate-90">
                          <IconChevronRight className="h-4 w-4" />
                          <span className="sr-only">Toggle</span>
                        </SidebarMenuAction>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {environments.map((env) => (
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
                      </CollapsibleContent>
                    </>
                  )}
                </SidebarMenuItem>
              </Collapsible>

              {/* Executors */}
              <Collapsible defaultOpen className="group/collapsible">
                <SidebarMenuItem>
                  <SidebarMenuButton asChild tooltip="Executors">
                    <Link href="/settings/executors">
                      <IconCpu className="h-4 w-4" />
                      <span>Executors</span>
                    </Link>
                  </SidebarMenuButton>
                  {executors.length > 0 && (
                    <>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuAction className="data-[state=open]:rotate-90">
                          <IconChevronRight className="h-4 w-4" />
                          <span className="sr-only">Toggle</span>
                        </SidebarMenuAction>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {executors.map((executor) => (
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
                      </CollapsibleContent>
                    </>
                  )}
                </SidebarMenuItem>
              </Collapsible>

              {/* Agents */}
              <Collapsible defaultOpen className="group/collapsible">
                <SidebarMenuItem>
                  <SidebarMenuButton asChild tooltip="Agents">
                    <Link href="/settings/agents">
                      <IconRobot className="h-4 w-4" />
                      <span>Agents</span>
                    </Link>
                  </SidebarMenuButton>
                  {agents.length > 0 && (
                    <>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuAction className="data-[state=open]:rotate-90">
                          <IconChevronRight className="h-4 w-4" />
                          <span className="sr-only">Toggle</span>
                        </SidebarMenuAction>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {agents.flatMap((agent) =>
                            agent.profiles.map((profile) => {
                              const encodedAgent = encodeURIComponent(agent.name);
                              const profilePath = `/settings/agents/${encodedAgent}/profiles/${profile.id}`;
                              return (
                              <SidebarMenuSubItem key={profile.id}>
                                <SidebarMenuSubButton
                                  asChild
                                  isActive={pathname === profilePath}
                                >
                                  <Link href={profilePath}>
                                    <span>{profile.name}</span>
                                  </Link>
                                </SidebarMenuSubButton>
                              </SidebarMenuSubItem>
                            );
                            })
                          )}
                        </SidebarMenuSub>
                      </CollapsibleContent>
                    </>
                  )}
                </SidebarMenuItem>
              </Collapsible>

              {/* Integrations */}
              <SidebarMenuItem>
                <SidebarMenuButton asChild isActive={pathname === '/settings/integrations'}>
                  <Link href="/settings/integrations">
                    <IconPlug className="h-4 w-4" />
                    <span>Integrations</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>

              {/* Plugins */}
              <SidebarMenuItem>
                <SidebarMenuButton asChild isActive={pathname === '/settings/plugins'}>
                  <Link href="/settings/plugins">
                    <IconPuzzle className="h-4 w-4" />
                    <span>Plugins</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  );
}
