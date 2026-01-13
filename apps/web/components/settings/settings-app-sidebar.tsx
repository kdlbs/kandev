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
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import { listEnvironmentsAction } from '@/app/actions/environments';
import { listExecutorsAction } from '@/app/actions/executors';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Environment, Executor } from '@/lib/types/http';
import { useAppStore } from '@/components/state-provider';

export function SettingsAppSidebar() {
  const pathname = usePathname();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const fallbackEnvironments = React.useMemo<Environment[]>(
    () =>
      SETTINGS_DATA.environments.map((env) => ({
        id: env.id,
        name: env.name,
        kind: env.kind,
        worktree_root: env.worktreeRoot,
        image_tag: env.imageTag,
        dockerfile: env.dockerfile,
        build_config: env.buildConfig
          ? {
              base_image: env.buildConfig.baseImage,
              install_agents: env.buildConfig.installAgents.join(','),
            }
          : undefined,
        created_at: '',
        updated_at: '',
      })),
    []
  );
  const fallbackExecutors = React.useMemo<Executor[]>(
    () =>
      SETTINGS_DATA.executors.map((executor) => ({
        id: executor.id,
        name: executor.name,
        type: executor.type,
        status: executor.status,
        is_system: executor.isSystem,
        config: executor.config,
        created_at: '',
        updated_at: '',
      })),
    []
  );
  const [environments, setEnvironments] = React.useState<Environment[]>(fallbackEnvironments);
  const [agents] = React.useState(SETTINGS_DATA.agents);
  const [executors, setExecutors] = React.useState<Executor[]>(fallbackExecutors);

  React.useEffect(() => {
    const client = getWebSocketClient();
    const envFallback = () => fallbackEnvironments;
    const execFallback = () => fallbackExecutors;

    if (client) {
      client
        .request<{ environments: Environment[] }>('environment.list', {})
        .then((resp) => setEnvironments(resp.environments))
        .catch(() => setEnvironments(envFallback()));
      client
        .request<{ executors: Executor[] }>('executor.list', {})
        .then((resp) => setExecutors(resp.executors))
        .catch(() => setExecutors(execFallback()));
      return;
    }

    listEnvironmentsAction()
      .then((resp) => setEnvironments(resp.environments))
      .catch(() => setEnvironments(envFallback()));
    listExecutorsAction()
      .then((resp) => setExecutors(resp.executors))
      .catch(() => setExecutors(execFallback()));
  }, [fallbackEnvironments, fallbackExecutors]);

  React.useEffect(() => {
    const client = getWebSocketClient();
    if (!client) {
      return;
    }

    const unsubscribeEnvironmentCreated = client.on('environment.created', (message) => {
      setEnvironments((prev) => {
        const exists = prev.some((env) => env.id === message.payload.id);
        const next = {
          id: message.payload.id,
          name: message.payload.name,
          kind: message.payload.kind,
          worktree_root: message.payload.worktree_root,
          image_tag: message.payload.image_tag,
          dockerfile: message.payload.dockerfile,
          build_config: message.payload.build_config,
          created_at: message.payload.created_at ?? '',
          updated_at: message.payload.updated_at ?? '',
        };
        return exists ? prev.map((env) => (env.id === next.id ? next : env)) : [next, ...prev];
      });
    });
    const unsubscribeEnvironmentUpdated = client.on('environment.updated', (message) => {
      setEnvironments((prev) =>
        prev.map((env) =>
          env.id === message.payload.id
            ? {
                ...env,
                name: message.payload.name,
                kind: message.payload.kind,
                worktree_root: message.payload.worktree_root,
                image_tag: message.payload.image_tag,
                dockerfile: message.payload.dockerfile,
                build_config: message.payload.build_config,
                updated_at: message.payload.updated_at ?? env.updated_at,
              }
            : env
        )
      );
    });
    const unsubscribeEnvironmentDeleted = client.on('environment.deleted', (message) => {
      setEnvironments((prev) => prev.filter((env) => env.id !== message.payload.id));
    });

    const unsubscribeExecutorCreated = client.on('executor.created', (message) => {
      setExecutors((prev) => {
        const exists = prev.some((executor) => executor.id === message.payload.id);
        const next = {
          id: message.payload.id,
          name: message.payload.name,
          type: message.payload.type,
          status: message.payload.status,
          is_system: message.payload.is_system,
          config: message.payload.config,
          created_at: message.payload.created_at ?? '',
          updated_at: message.payload.updated_at ?? '',
        };
        return exists
          ? prev.map((executor) => (executor.id === next.id ? next : executor))
          : [next, ...prev];
      });
    });
    const unsubscribeExecutorUpdated = client.on('executor.updated', (message) => {
      setExecutors((prev) =>
        prev.map((executor) =>
          executor.id === message.payload.id
            ? {
                ...executor,
                name: message.payload.name,
                type: message.payload.type,
                status: message.payload.status,
                config: message.payload.config,
                updated_at: message.payload.updated_at ?? executor.updated_at,
              }
            : executor
        )
      );
    });
    const unsubscribeExecutorDeleted = client.on('executor.deleted', (message) => {
      setExecutors((prev) => prev.filter((executor) => executor.id !== message.payload.id));
    });

    return () => {
      unsubscribeEnvironmentCreated();
      unsubscribeEnvironmentUpdated();
      unsubscribeEnvironmentDeleted();
      unsubscribeExecutorCreated();
      unsubscribeExecutorUpdated();
      unsubscribeExecutorDeleted();
    };
  }, []);

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
