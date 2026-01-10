'use client';

import * as React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  IconSettings,
  IconFolder,
  IconServer,
  IconRobot,
  IconChevronRight,
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
} from '@/components/ui/sidebar';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';

export function SettingsAppSidebar() {
  const pathname = usePathname();
  const [workspaces] = React.useState(SETTINGS_DATA.workspaces);
  const [environments] = React.useState(SETTINGS_DATA.environments);
  const [agents] = React.useState(SETTINGS_DATA.agents);

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
              <SidebarMenuItem>
                <SidebarMenuButton asChild isActive={pathname === '/settings/general'}>
                  <Link href="/settings/general">
                    <IconSettings className="h-4 w-4" />
                    <span>General</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>

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
                          {workspaces.map((workspace) => (
                            <SidebarMenuSubItem key={workspace.id}>
                              <SidebarMenuSubButton
                                asChild
                                isActive={pathname === `/settings/workspace/${workspace.id}`}
                              >
                                <Link href={`/settings/workspace/${workspace.id}`}>
                                  <span>{workspace.name}</span>
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
                          {agents.map((agent) => (
                            <SidebarMenuSubItem key={agent.id}>
                              <SidebarMenuSubButton
                                asChild
                                isActive={pathname === `/settings/agent/${agent.id}`}
                              >
                                <Link href={`/settings/agent/${agent.id}`}>
                                  <span>{agent.name}</span>
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
