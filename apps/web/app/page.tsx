import { PageClient } from '@/app/page-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchWorkflowSnapshot, fetchUserSettings, listWorkflows, listRepositories, listWorkspaces, listTaskSessionMessages } from '@/lib/api';
import { snapshotToState } from '@/lib/ssr/mapper';
import type { AppState } from '@/lib/state/store';

// Server Component: runs on the server for SSR and data hydration.
type PageProps = {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
};

export default async function Page({ searchParams }: PageProps) {
  try {
    const resolvedParams = searchParams ? await searchParams : {};
    const workspaceParam = resolvedParams.workspaceId;
    const workflowParam = resolvedParams.workflowId;
    const taskIdParam = resolvedParams.taskId;
    const sessionIdParam = resolvedParams.sessionId;
    const workspaceId = Array.isArray(workspaceParam) ? workspaceParam[0] : workspaceParam;
    const workflowIdParam = Array.isArray(workflowParam) ? workflowParam[0] : workflowParam;
    const taskId = Array.isArray(taskIdParam) ? taskIdParam[0] : taskIdParam;
    const sessionId = Array.isArray(sessionIdParam) ? sessionIdParam[0] : sessionIdParam;

    const [workspaces, userSettingsResponse] = await Promise.all([
      listWorkspaces({ cache: 'no-store' }),
      fetchUserSettings({ cache: 'no-store' }).catch(() => null),
    ]);
    const userSettings = userSettingsResponse?.settings;
    const settingsWorkspaceId = userSettings?.workspace_id || null;
    const settingsWorkflowId = userSettings?.workflow_filter_id || null;
    const settingsRepositoryIds = Array.from(new Set(userSettings?.repository_ids ?? [])).sort();
    const settingsEnablePreviewOnClick = userSettings?.enable_preview_on_click ?? false;
    const activeWorkspaceId =
      workspaces.workspaces.find((workspace) => workspace.id === workspaceId)?.id ??
      workspaces.workspaces.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
      workspaces.workspaces[0]?.id ??
      null;
    let initialState: Partial<AppState> = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
          description: workspace.description ?? null,
          owner_id: workspace.owner_id,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
          created_at: workspace.created_at,
          updated_at: workspace.updated_at,
        })),
        activeId: activeWorkspaceId,
      },
      userSettings: {
        workspaceId: activeWorkspaceId,
        workflowId: settingsWorkflowId,
        kanbanViewMode: userSettings?.kanban_view_mode || null,
        repositoryIds: settingsRepositoryIds,
        preferredShell: userSettings?.preferred_shell || null,
        shellOptions: userSettingsResponse?.shell_options ?? [],
        defaultEditorId: userSettings?.default_editor_id || null,
        enablePreviewOnClick: settingsEnablePreviewOnClick,
        chatSubmitKey: userSettings?.chat_submit_key ?? 'cmd_enter',
        reviewAutoMarkOnScroll: userSettings?.review_auto_mark_on_scroll ?? true,
        lspAutoStartLanguages: userSettings?.lsp_auto_start_languages ?? [],
        lspAutoInstallLanguages: userSettings?.lsp_auto_install_languages ?? [],
        lspServerConfigs: userSettings?.lsp_server_configs ?? {},
        loaded: Boolean(userSettings),
      },
    };

    if (!activeWorkspaceId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const [workflowList, repositoriesResponse] = await Promise.all([
      listWorkflows(activeWorkspaceId, { cache: 'no-store' }),
      listRepositories(activeWorkspaceId, undefined, { cache: 'no-store' }).catch(() => ({ repositories: [] })),
    ]);

    // Use workflowIdParam if it exists in this workspace, otherwise fall back
    // Client-side code will handle task selection even if workflow doesn't match
    const workflowId =
      workflowList.workflows.find((workflow) => workflow.id === workflowIdParam)?.id ??
      workflowList.workflows.find((workflow) => workflow.id === settingsWorkflowId)?.id ??
      workflowList.workflows[0]?.id;
    initialState = {
      ...initialState,
      userSettings: {
        ...(initialState.userSettings as AppState['userSettings']),
        workflowId: workflowId ?? null,
      },
      workflows: {
        items: workflowList.workflows.map((workflow) => ({
          id: workflow.id,
          workspaceId: workflow.workspace_id,
          name: workflow.name,
        })),
        activeId: workflowId ?? null,
      },
      repositories: {
        itemsByWorkspaceId: { [activeWorkspaceId]: repositoriesResponse.repositories },
        loadingByWorkspaceId: { [activeWorkspaceId]: false },
        loadedByWorkspaceId: { [activeWorkspaceId]: true },
      },
    };

    if (!workflowId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const snapshot = await fetchWorkflowSnapshot(workflowId, { cache: 'no-store' });
    initialState = { ...initialState, ...snapshotToState(snapshot) };

    // Load messages for selected task session if provided (SSR optimization)
    if (taskId && sessionId) {
      try {
        // Load most recent messages in descending order, then reverse to show oldest-to-newest
        const messagesResponse = await listTaskSessionMessages(sessionId, { limit: 50, sort: 'desc' }, { cache: 'no-store' });
        const messages = [...(messagesResponse.messages ?? [])].reverse();
        initialState = {
          ...initialState,
          messages: {
            bySession: {
              [sessionId]: messages,
            },
            metaBySession: {
              [sessionId]: {
                isLoading: false,
                hasMore: messagesResponse.has_more ?? false,
                // oldestCursor should be the first (oldest) message ID for lazy loading older messages
                oldestCursor: messages[0]?.id ?? null,
              },
            },
          },
        };
      } catch (error) {
        // SSR failed - client will load messages via WebSocket
        console.warn('Could not SSR messages (client will load via WebSocket):', error instanceof Error ? error.message : String(error));
      }
    }

    return (
      <>
        <StateHydrator initialState={initialState} />
        <PageClient initialTaskId={taskId} initialSessionId={sessionId} />
      </>
    );
  } catch {
    return <PageClient />;
  }
}
