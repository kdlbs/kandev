import { PageClient } from "@/app/page-client";
import { StateHydrator } from "@/components/state-hydrator";
import {
  fetchWorkflowSnapshot,
  fetchUserSettings,
  listWorkflows,
  listRepositories,
  listWorkspaces,
  listTaskSessionMessages,
  listQuickChatSessions,
} from "@/lib/api";
import { listWorkspaceTaskPRs } from "@/lib/api/domains/github-api";
import { snapshotToState } from "@/lib/ssr/mapper";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { AppState } from "@/lib/state/store";
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";

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
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

function buildUserSettingsState(
  resp: UserSettingsResponse | null,
  workspaceId: string | null,
): AppState["userSettings"] {
  return { ...mapUserSettingsResponse(resp), workspaceId };
}

function readAgentProfileId(
  metadata: Record<string, unknown> | null | undefined,
): string | undefined {
  if (!metadata || typeof metadata !== "object") return undefined;
  const value = metadata.agent_profile_id;
  return typeof value === "string" ? value : undefined;
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

    // Fire-and-forget: warm the backend PR cache for this workspace.
    // The client will fetch the data after mount via useWorkspacePRs.
    listWorkspaceTaskPRs(activeWorkspaceId, { cache: "no-store" }).catch(() => {});

    const [workflowList, repositoriesResponse, quickChatResponse] = await Promise.all([
      listWorkflows(activeWorkspaceId, { cache: "no-store", includeHidden: true }),
      listRepositories(activeWorkspaceId, undefined, { cache: "no-store" }).catch(() => ({
        repositories: [],
      })),
      listQuickChatSessions(activeWorkspaceId, { cache: "no-store" }).catch(() => ({ tasks: [] })),
    ]);

    // Active workflow defaults to the first non-hidden workflow when no preference is set,
    // so hidden workflows (e.g., improve-kandev) never auto-select on load.
    const workflowId = resolveActiveId(
      workflowList.workflows.filter((w) => !w.hidden),
      workflowIdParam,
      settingsWorkflowId,
    );

    // Map quick chat tasks to sessions
    const quickChatSessions = quickChatResponse.tasks
      .filter((t) => t.primary_session_id) // Only include tasks with active sessions
      .map((t) => ({
        sessionId: t.primary_session_id!,
        workspaceId: t.workspace_id,
        name: t.title !== "Quick Chat" ? t.title : undefined,
        agentProfileId: readAgentProfileId(t.metadata),
      }));

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
          hidden: w.hidden,
        })),
        activeId: workflowId,
      },
      repositories: {
        itemsByWorkspaceId: { [activeWorkspaceId]: repositoriesResponse.repositories },
        loadingByWorkspaceId: { [activeWorkspaceId]: false },
        loadedByWorkspaceId: { [activeWorkspaceId]: true },
      },
      quickChat: {
        isOpen: false,
        sessions: quickChatSessions,
        activeSessionId: null,
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
