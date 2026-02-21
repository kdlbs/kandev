import { PageClient } from "@/app/page-client";
import { StateHydrator } from "@/components/state-hydrator";
import {
  fetchWorkflowSnapshot,
  fetchUserSettings,
  listWorkflows,
  listRepositories,
  listWorkspaces,
  listTaskSessionMessages,
} from "@/lib/api";
import { snapshotToState } from "@/lib/ssr/mapper";
import type { AppState } from "@/lib/state/store";
import type { ListWorkspacesResponse, UserSettings, UserSettingsResponse } from "@/lib/types/http";

// Server Component: runs on the server for SSR and data hydration.
type PageProps = {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
};

function resolveParam(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];
function mapWorkspaceItem(ws: WorkspaceItem) {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

function buildLspConfig(s: UserSettings) {
  return {
    lspAutoStartLanguages: s.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: s.lsp_auto_install_languages ?? [],
    lspServerConfigs: s.lsp_server_configs ?? {},
  };
}

function buildUserSettingsState(
  resp: UserSettingsResponse | null,
  workspaceId: string | null,
): AppState["userSettings"] {
  const s = resp?.settings;
  const shellOptions = resp ? (resp.shell_options ?? []) : [];
  if (!s) {
    return {
      workspaceId,
      workflowId: null,
      kanbanViewMode: null,
      repositoryIds: [],
      preferredShell: null,
      shellOptions,
      defaultEditorId: null,
      enablePreviewOnClick: false,
      chatSubmitKey: "cmd_enter",
      reviewAutoMarkOnScroll: true,
      savedLayouts: [],
      lspAutoStartLanguages: [],
      lspAutoInstallLanguages: [],
      lspServerConfigs: {},
      loaded: false,
    };
  }
  return {
    workspaceId,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: Array.from(new Set(s.repository_ids ?? [])).sort(),
    preferredShell: s.preferred_shell || null,
    shellOptions,
    defaultEditorId: s.default_editor_id || null,
    enablePreviewOnClick: s.enable_preview_on_click ?? false,
    chatSubmitKey: s.chat_submit_key ?? "cmd_enter",
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? true,
    savedLayouts: s.saved_layouts ?? [],
    ...buildLspConfig(s),
    loaded: true,
  };
}

function resolveActiveId<T extends { id: string }>(
  items: T[],
  preferredId?: string,
  fallbackId?: string | null,
): string | null {
  return (
    items.find((i) => i.id === preferredId)?.id ??
    items.find((i) => i.id === fallbackId)?.id ??
    items[0]?.id ??
    null
  );
}

function buildBaseState(
  workspaces: ListWorkspacesResponse,
  userSettingsResponse: UserSettingsResponse | null,
  activeWorkspaceId: string | null,
): Partial<AppState> {
  return {
    workspaces: {
      items: workspaces.workspaces.map(mapWorkspaceItem),
      activeId: activeWorkspaceId,
    },
    userSettings: buildUserSettingsState(userSettingsResponse, activeWorkspaceId),
  };
}

async function loadSnapshotState(
  workflowId: string,
  taskId: string | undefined,
  sessionId: string | undefined,
): Promise<Partial<AppState>> {
  const [snapshot, messagesResponse] = await Promise.all([
    fetchWorkflowSnapshot(workflowId, { cache: "no-store" }),
    taskId && sessionId
      ? listTaskSessionMessages(
          sessionId,
          { limit: 50, sort: "desc" },
          { cache: "no-store" },
        ).catch(() => null)
      : Promise.resolve(null),
  ]);
  const state: Partial<AppState> = { ...snapshotToState(snapshot) };

  if (sessionId && messagesResponse) {
    const messages = [...(messagesResponse.messages ?? [])].reverse();
    state.messages = {
      bySession: { [sessionId]: messages },
      metaBySession: {
        [sessionId]: {
          isLoading: false,
          hasMore: messagesResponse.has_more ?? false,
          oldestCursor: messages[0]?.id ?? null,
        },
      },
    };
  }
  return state;
}

export default async function Page({ searchParams }: PageProps) {
  try {
    const resolvedParams = searchParams ? await searchParams : {};
    const workspaceId = resolveParam(resolvedParams.workspaceId);
    const workflowIdParam = resolveParam(resolvedParams.workflowId);
    const taskId = resolveParam(resolvedParams.taskId);
    const sessionId = resolveParam(resolvedParams.sessionId);

    const [workspaces, userSettingsResponse] = await Promise.all([
      listWorkspaces({ cache: "no-store" }),
      fetchUserSettings({ cache: "no-store" }).catch(() => null),
    ]);
    const settingsWorkspaceId = userSettingsResponse?.settings?.workspace_id || null;
    const settingsWorkflowId = userSettingsResponse?.settings?.workflow_filter_id || null;
    const activeWorkspaceId = resolveActiveId(
      workspaces.workspaces,
      workspaceId,
      settingsWorkspaceId,
    );

    let initialState = buildBaseState(workspaces, userSettingsResponse, activeWorkspaceId);

    if (!activeWorkspaceId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const [workflowList, repositoriesResponse] = await Promise.all([
      listWorkflows(activeWorkspaceId, { cache: "no-store" }),
      listRepositories(activeWorkspaceId, undefined, { cache: "no-store" }).catch(() => ({
        repositories: [],
      })),
    ]);

    const workflowId = resolveActiveId(workflowList.workflows, workflowIdParam, settingsWorkflowId);

    initialState = {
      ...initialState,
      userSettings: {
        ...(initialState.userSettings as AppState["userSettings"]),
        workflowId,
      },
      workflows: {
        items: workflowList.workflows.map((w) => ({
          id: w.id,
          workspaceId: w.workspace_id,
          name: w.name,
        })),
        activeId: workflowId,
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

    const snapshotState = await loadSnapshotState(workflowId, taskId, sessionId);
    initialState = { ...initialState, ...snapshotState };

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
