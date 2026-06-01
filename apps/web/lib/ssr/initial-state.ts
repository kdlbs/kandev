import type { AppState } from "@/lib/state/store";
import type {
  Agent,
  AgentDiscovery,
  AvailableAgent,
  Executor,
  ListRepositoriesResponse,
  RepositoryScript,
  TaskSession,
  ToolStatus,
} from "@/lib/types/http";
import type { KanbanState, WorkflowsState } from "@/lib/state/slices/kanban/types";
import type { AgentProfileOption, UserSettingsState } from "@/lib/types/settings";

/**
 * Structural workspace shape produced by SSR builders. Intentionally uses plain
 * `string` ids (not the branded `WorkspaceId`) because the various SSR sources
 * (kanban list, office list) map to slightly different shapes. The hydrator
 * casts these into the TQ `Workspace` cache shape when seeding.
 */
export type SsrWorkspaceItem = {
  id: string;
  name: string;
  description?: string | null;
  owner_id: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
  default_config_agent_profile_id?: string | null;
  created_at: string;
  updated_at: string;
};

/**
 * The SSR snapshot handed to `StateHydrator`. It is a `Partial<AppState>`
 * (client state hydrated into Zustand) PLUS the workspace-domain server data
 * (`workspaces.items`, `repositories.*`) that no longer lives in Zustand but is
 * still produced by SSR builders and used to seed the TanStack Query cache.
 *
 * Keep the workspace/repository shapes here so SSR page builders and the
 * hydrator agree without re-introducing the deleted Zustand slice fields.
 */
export type SsrInitialState = Omit<Partial<AppState>, "workspaces" | "workflows"> & {
  /**
   * Settings-domain server data produced by SSR builders. No longer hydrated
   * into a Zustand slice — seeded into the TanStack Query cache by the
   * hydrator (`qk.settings.agents/agentProfiles/userSettings`).
   */
  settingsAgents?: { items: Agent[] };
  agentProfiles?: { items: AgentProfileOption[]; version?: number };
  executors?: { items: Executor[] };
  agentDiscovery?: { items: AgentDiscovery[] };
  availableAgents?: { items: AvailableAgent[]; tools: ToolStatus[] };
  /** Mapped user settings, seeded into `qk.settings.userSettings()`. */
  userSettings?: UserSettingsState;
  /**
   * Session-domain server data produced by SSR builders. No longer hydrated
   * into a Zustand slice — seeded into the TanStack Query cache by the
   * hydrator (`qk.taskSession.byId` / `qk.taskSession.byTask`).
   */
  taskSessions?: { items: Record<string, TaskSession> };
  taskSessionsByTask?: { itemsByTaskId: Record<string, TaskSession[]> };
  workspaces?: {
    activeId?: string | null;
    items?: SsrWorkspaceItem[];
  };
  /**
   * The active task's single-workflow snapshot, produced by SSR. Server data —
   * seeded into the TanStack Query `qk.kanban.multi()` cache by the hydrator,
   * no longer hydrated into a Zustand slice.
   */
  kanban?: KanbanState;
  /**
   * The workspace workflows list (+ optional active selection), produced by SSR.
   * `items` is server data seeded into `qk.kanban.workflowsList()` by the
   * hydrator; `activeId` is client-only selection state that stays in Zustand.
   */
  workflows?: { activeId?: string | null; items?: WorkflowsState["items"] };
  repositories?: {
    itemsByWorkspaceId?: Record<string, ListRepositoriesResponse["repositories"]>;
    loadingByWorkspaceId?: Record<string, boolean>;
    loadedByWorkspaceId?: Record<string, boolean>;
  };
  repositoryScripts?: {
    itemsByRepositoryId?: Record<string, RepositoryScript[]>;
    loadingByRepositoryId?: Record<string, boolean>;
    loadedByRepositoryId?: Record<string, boolean>;
  };
};
