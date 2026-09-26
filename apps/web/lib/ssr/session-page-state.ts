import {
  fetchWorkflowSnapshot,
  fetchTask,
  fetchUserSettings,
  listAgents,
  listWorkflows,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
  listWorkspaces,
} from "@/lib/api";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import { fetchTerminals } from "@/lib/api/domains/user-shell-api";
import type {
  ListMessagesResponse,
  Task,
  TaskSession,
  UserSettingsResponse,
  WorkflowSnapshot,
} from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { snapshotToState, taskToState } from "@/lib/ssr/mapper";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import { latestIncompleteTurnId } from "@/lib/state/slices/session/turn-actions";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";
import type { AppState } from "@/lib/state/store";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";
// Aliased: `t` is the Terminal parameter name throughout this module.
import { t as translate } from "@/lib/i18n";

export const OPTIONAL_HYDRATION_TIMEOUT_MS = 5_000;

type OptionalHydrationResult<T> = { status: "fulfilled"; value: T } | { status: "unavailable" };

/**
 * Starts an optional-hydration window: a shared deadline timer bounds every
 * `load()` issued before `complete()` fires, so optional SSR fetches can never
 * extend route loading. Returns the load/complete handle for those fetches.
 */
function beginOptionalHydration() {
  let deadlineTimer: ReturnType<typeof setTimeout>;
  const deadline = new Promise<void>((resolve) => {
    deadlineTimer = setTimeout(resolve, OPTIONAL_HYDRATION_TIMEOUT_MS);
  });

  return {
    /**
     * Runs an optional fetch, resolving with its value if it succeeds before the
     * shared deadline and with "unavailable" (after a console warning) on failure
     * or timeout, so callers can seed state without those slices.
     */
    load<T>(label: string, operation: () => Promise<T>): Promise<OptionalHydrationResult<T>> {
      return new Promise((resolve) => {
        let settled = false;
        /** First-call-wins resolver for the load promise; later calls are ignored. */
        const settle = (value: OptionalHydrationResult<T>) => {
          if (settled) return;
          settled = true;
          resolve(value);
        };

        void Promise.resolve()
          .then(operation)
          .then(
            (value) => settle({ status: "fulfilled", value }),
            (error) => {
              if (settled) return;
              console.warn(
                `[session-page-state] optional ${label} failed; continuing without it`,
                error,
              );
              settle({ status: "unavailable" });
            },
          );
        void deadline.then(() => {
          if (!settled) {
            console.warn(
              `[session-page-state] optional ${label} timed out after ${OPTIONAL_HYDRATION_TIMEOUT_MS}ms; continuing without it`,
            );
            settle({ status: "unavailable" });
          }
        });
      });
    },
    /** Cancels the shared deadline timer once all optional fetches are in flight. */
    complete() {
      clearTimeout(deadlineTimer);
    },
  };
}

/** Unwraps a fulfilled optional result; returns undefined when it was unavailable. */
function optionalValue<T>(result: OptionalHydrationResult<T>): T | undefined {
  return result.status === "fulfilled" ? result.value : undefined;
}

/**
 * Builds a Partial<AppState> from an optional value, returning {} when the value
 * is missing so unavailable slices contribute nothing to the initial state.
 */
function optionalState<T>(
  value: T | undefined,
  build: (resolved: T) => Partial<AppState>,
): Partial<AppState> {
  return value === undefined ? {} : build(value);
}

/**
 * Builds the worktrees and sessionWorktreesBySessionId state slices from the
 * sessions that carry a worktree_id.
 */
function buildWorktreeState(allSessions: TaskSession[]) {
  const sessionsWithWorktrees = allSessions.filter((s) => s.worktree_id);
  return {
    worktrees: {
      items: Object.fromEntries(
        sessionsWithWorktrees.map((s) => [
          s.worktree_id,
          {
            id: s.worktree_id!,
            sessionId: s.id,
            repositoryId: s.repository_id ?? undefined,
            path: s.worktree_path ?? undefined,
            branch: s.worktree_branch ?? undefined,
          },
        ]),
      ),
    },
    sessionWorktreesBySessionId: {
      itemsBySessionId: Object.fromEntries(
        sessionsWithWorktrees.map((s) => [s.id, [s.worktree_id!]]),
      ),
    },
  };
}

type BuildSessionPageStateParams = {
  task: Task;
  sessionId: string | null;
  snapshot?: Awaited<ReturnType<typeof fetchWorkflowSnapshot>>;
  agents?: Awaited<ReturnType<typeof listAgents>>;
  repositories?: Awaited<ReturnType<typeof listRepositories>>["repositories"];
  allSessions: TaskSession[];
  // Full session payload (with agent_profile_snapshot) for the active sessionId,
  // when available. The list endpoint returns lightweight summaries without the
  // snapshot, which would force the model selector to fall back to the agent's
  // default model on SSR — visible as a brief flash of the wrong model before
  // the WS-driven cached state arrives.
  activeSession: TaskSession | null;
  workspaces?: Awaited<ReturnType<typeof listWorkspaces>>["workspaces"];
  workflows?: Awaited<ReturnType<typeof listWorkflows>>["workflows"];
  turns?: Awaited<ReturnType<typeof listSessionTurns>>["turns"];
  userSettingsResponse?: UserSettingsResponse | null;
  messagesResponse?: ListMessagesResponse | null;
};

/**
 * Composes the full SSR initial state for a session page: task/message state
 * plus the resource, session, worktree, prepare-progress, agent, and user
 * settings slices, each contributed only when the corresponding data loaded.
 */
function buildSessionPageState(p: BuildSessionPageStateParams) {
  const { task, sessionId, snapshot, agents, allSessions, messagesResponse } = p;
  const messages = messagesResponse?.messages ? [...messagesResponse.messages].reverse() : [];
  const taskState =
    messagesResponse === undefined
      ? taskToState(task, sessionId)
      : taskToState(task, sessionId, {
          items: messages,
          hasMore: messagesResponse?.has_more ?? false,
          oldestCursor: messages[0]?.id ?? null,
        });

  return {
    ...optionalState(snapshot, snapshotToState),
    ...taskState,
    ...buildResourceState(p),
    ...buildSessionState(p),
    ...buildWorktreeState(allSessions),
    ...buildPrepareProgressState(allSessions),
    ...optionalState(agents, (value) => ({
      settingsAgents: { items: value.agents },
      settingsData: { agentsLoaded: true, executorsLoaded: false },
    })),
    ...optionalState(p.userSettingsResponse, (value) => ({
      userSettings: mapUserSettingsResponse(value),
    })),
  };
}

/** Builds the session-page hydration slice for repositories, agents, and workflow resources. */
function buildResourceState(p: BuildSessionPageStateParams) {
  const { task, agents, repositories, workspaces, workflows } = p;
  const repositoryId = task.repositories?.[0]?.repository_id;
  const repository = repositories?.find((r) => r.id === repositoryId);
  const scripts = repository?.scripts ?? [];
  return {
    workspaces: {
      ...(workspaces ? { items: workspaces.map(mapWorkspaceItem) } : {}),
      activeId: task.workspace_id,
    } as Partial<AppState>["workspaces"],
    // Don't write activeId — null means "All Workflows"; task context lives in kanban.workflowId.
    ...optionalState(workflows, (value) => ({
      workflows: {
        items: value.map((w) => ({
          id: w.id as string,
          workspaceId: w.workspace_id as string,
          name: w.name,
          hidden: w.hidden,
          style: w.style,
        })),
      } as Partial<AppState>["workflows"],
    })),
    ...optionalState(repositories, (value) => ({
      repositories: {
        itemsByWorkspaceId: { [task.workspace_id]: value },
        loadingByWorkspaceId: { [task.workspace_id]: false },
        loadedByWorkspaceId: { [task.workspace_id]: true },
      },
    })),
    ...(repositories
      ? {
          repositoryScripts: repositoryId
            ? {
                itemsByRepositoryId: { [repositoryId]: scripts },
                loadingByRepositoryId: { [repositoryId]: false },
                loadedByRepositoryId: { [repositoryId]: true },
              }
            : { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
        }
      : {}),
    ...optionalState(agents, (value) => ({
      agentProfiles: {
        items: value.agents.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        ),
        version: 0,
      },
    })),
  };
}

/** Builds the session-page hydration slice (task sessions, turns, models) for the page's task. */
function buildSessionState(p: BuildSessionPageStateParams) {
  const { task, sessionId, allSessions, activeSession, turns } = p;
  // Prefer the full active session payload (with agent_profile_snapshot) over
  // its summary entry in allSessions so the model selector can resolve the
  // persisted model on first render without flashing the agent default.
  const itemsBySessionId = Object.fromEntries(allSessions.map((s) => [s.id, s]));
  if (activeSession?.id) {
    itemsBySessionId[activeSession.id] = activeSession;
  }
  return {
    taskSessions: { items: itemsBySessionId },
    ...buildSessionModelsState(activeSession),
    taskSessionsByTask: {
      itemsByTaskId: { [task.id]: allSessions },
      loadingByTaskId: { [task.id]: false },
      loadedByTaskId: { [task.id]: true },
    },
    ...(turns !== undefined
      ? {
          turns: sessionId
            ? {
                bySession: { [sessionId]: turns },
                activeBySession: {
                  // Same timestamp-aware selection as the store's
                  // reconciliation (latestIncompleteTurnId compares
                  // started_at with nanosecond precision) — not array
                  // position, which a non-chronological API response
                  // would misorder.
                  [sessionId]: latestIncompleteTurnId(turns) ?? null,
                },
                // The SSR turn list is the session's complete persisted
                // history — mark it loaded so turn-derived UI never
                // re-fetches or mistakes WS-seeded live turns for it.
                loadedBySession: { [sessionId]: true },
                reconcileEpochBySession: {},
                settledBoundaryBySession: {},
              }
            : {
                bySession: {},
                activeBySession: {},
                loadedBySession: {},
                reconcileEpochBySession: {},
                settledBoundaryBySession: {},
              },
        }
      : {}),
    environmentIdBySessionId: Object.fromEntries(
      allSessions.filter((s) => s.task_environment_id).map((s) => [s.id, s.task_environment_id!]),
    ),
  };
}

/** Returns the value narrowed to its string-valued entries, or undefined when none qualify. */
function stringMap(value: unknown): Record<string, string> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string",
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

/** Returns the value as a plain record when it is a non-array object; otherwise undefined. */
function objectMap(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

/** Returns the value when it is a string; otherwise undefined. */
function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

/** Filters an array down to its plain-object items, or returns [] for non-arrays. */
function objectList(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
    : [];
}

/** Maps a raw ACP model record to the display shape, defaulting missing strings to "". */
function mapSessionModel(model: Record<string, unknown>) {
  return {
    modelId: stringValue(model.model_id) ?? "",
    name: stringValue(model.name) ?? "",
    description: stringValue(model.description),
    usageMultiplier: stringValue(model.usage_multiplier),
  };
}

/**
 * Maps a raw ACP config option, preferring the runtime (or override) value over
 * the option's own current_value and defaulting type to "select" when absent.
 */
function mapSessionConfigOption(
  option: Record<string, unknown>,
  runtimeOptions: Record<string, string>,
) {
  const id = stringValue(option.id) ?? "";
  return {
    type: stringValue(option.type) ?? "select",
    id,
    name: stringValue(option.name) ?? "",
    description: stringValue(option.description),
    currentValue: runtimeOptions[id] ?? stringValue(option.current_value) ?? "",
    category: stringValue(option.category),
    options: Array.isArray(option.options)
      ? (option.options as { value: string; name: string; description?: string }[])
      : undefined,
  };
}

/**
 * Extracts the ACP model-state snapshot, runtime config, and overrides from a
 * session's metadata; returns {} when the session or its snapshot is missing.
 */
function sessionModelHydrationMetadata(session: TaskSession | null) {
  if (!session) return {};
  const snapshot = objectMap(session.metadata?.acp_model_state);
  if (!snapshot) return {};
  return {
    session,
    snapshot,
    runtime: objectMap(session.metadata?.runtime_config),
    overrides: objectMap(session.metadata?.runtime_config_overrides),
  };
}

/**
 * Builds the sessionModels slice (current model, available models, config
 * options) for a session from its ACP hydration metadata; {} when nothing
 * resolvable exists.
 */
function buildSessionModelsState(session: TaskSession | null) {
  const metadata = sessionModelHydrationMetadata(session);
  if (!metadata.session || !metadata.snapshot) return {};
  const { snapshot, runtime, overrides } = metadata;
  const runtimeOptions = {
    ...stringMap(runtime?.config_options),
    ...stringMap(overrides?.config_options),
  };
  const currentModelId =
    stringValue(overrides?.model) ??
    stringValue(runtime?.model) ??
    stringValue(snapshot.current_model_id) ??
    "";
  const models = objectList(snapshot.models)
    .map(mapSessionModel)
    .filter((model) => !!model.modelId);
  const configOptions = objectList(snapshot.config_options)
    .map((option) => mapSessionConfigOption(option, runtimeOptions))
    .filter((option) => !!option.id);
  if (!currentModelId && models.length === 0 && configOptions.length === 0) return {};

  return {
    sessionModels: {
      bySessionId: {
        [metadata.session.id]: {
          currentModelId,
          models,
          configOptions,
          configOptionsSettled: snapshot.config_options_settled === true,
          configBaseline: stringMap(metadata.session.metadata?.acp_config_baseline),
        },
      },
    },
  };
}

/** Builds the prepareProgress slice from each session's prepare metadata; {} when none exists. */
function buildPrepareProgressState(allSessions: TaskSession[]) {
  const bySessionId: Record<string, SessionPrepareState> = {};

  for (const session of allSessions) {
    const prepareState = prepareResultToSessionState(session.id, session.metadata);
    if (prepareState) bySessionId[session.id] = prepareState;
  }

  if (Object.keys(bySessionId).length === 0) return {};
  return { prepareProgress: { bySessionId } };
}

export type FetchedSessionData = {
  task: Task;
  sessionId: string | null;
  initialState: ReturnType<typeof taskToState>;
  initialTerminals: Terminal[];
};

/**
 * SSR entry point for a session route: fetches the full session, its task, and
 * the task's session list, then runs the shared optional-enrichment pipeline to
 * produce the initial page state.
 */
export async function fetchSessionData(sessionId: string): Promise<FetchedSessionData> {
  const { fetchTaskSession } = await import("@/lib/api");
  const sessionResponse = await fetchTaskSession(sessionId, { cache: "no-store" });
  const activeSession = sessionResponse.session ?? null;
  if (!activeSession?.task_id) throw new Error("No task_id found for session");
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(activeSession.task_id, { cache: "no-store" }),
    listTaskSessions(activeSession.task_id, { cache: "no-store" }),
  ]);

  const optionalHydration = beginOptionalHydration();
  return fetchSessionDataFromTask(
    task,
    sessionId,
    allSessionsResponse,
    Promise.resolve({ status: "fulfilled", value: { session: activeSession } }),
    optionalHydration,
  );
}

/**
 * SSR entry point for a task route: resolves the primary (or first) session,
 * seeding task-only data when no session exists yet and otherwise enriching via
 * the shared optional-hydration pipeline.
 */
export async function fetchSessionDataForTask(taskId: string): Promise<FetchedSessionData> {
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listTaskSessions(taskId, { cache: "no-store" }),
  ]);
  const sessions = allSessionsResponse.sessions ?? [];

  const sessionId = task.primary_session_id ?? sessions[0]?.id;
  if (!sessionId) {
    // No sessions yet — fetch task/workspace data so the store is seeded and
    // the auto-start hook can fire immediately without a client-side crash.
    return fetchTaskDataOnly(task, allSessionsResponse);
  }

  // Refetch the active session via the single-session endpoint to get
  // agent_profile_snapshot, which the list endpoint strips. See
  // BuildSessionPageStateParams.activeSession for the SSR-flicker rationale.
  // All remaining enrichment shares this deadline so no optional request can
  // extend route loading beyond the configured bound.
  const optionalHydration = beginOptionalHydration();
  const { fetchTaskSession } = await import("@/lib/api");
  const activeSessionResponse = optionalHydration.load("active session snapshot", () =>
    fetchTaskSession(sessionId, { cache: "no-store" }),
  );
  return fetchSessionDataFromTask(
    task,
    sessionId,
    allSessionsResponse,
    activeSessionResponse,
    optionalHydration,
  );
}

/**
 * Builds SSR data for a task with no sessions yet: fetches the optional
 * convenience slices (snapshot, agents, repositories, workspaces, workflows,
 * user settings) and seeds state with sessionId null so the auto-start hook can
 * fire immediately without a client-side crash.
 */
async function fetchTaskDataOnly(
  task: Task,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
): Promise<FetchedSessionData> {
  const optionalHydration = beginOptionalHydration();
  const results = await Promise.all([
    // Only the task and its session list are essential to route recovery. These
    // enrichment requests seed convenience state and must never strand the route.
    optionalHydration.load("workflow snapshot", () =>
      task.workflow_id
        ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
        : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    ),
    optionalHydration.load("agents", () => listAgents({ cache: "no-store" })),
    optionalHydration.load("repositories", () =>
      listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    ),
    optionalHydration.load("workspaces", () => listWorkspaces({ cache: "no-store" })),
    optionalHydration.load("workflows", () =>
      listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }),
    ),
    optionalHydration.load("user settings", () => fetchUserSettings({ cache: "no-store" })),
  ]);
  optionalHydration.complete();
  const [
    snapshot,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    userSettingsResponse,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const snapshotValue = optionalValue(snapshot);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;

  const initialState = buildSessionPageState({
    task,
    sessionId: null,
    snapshot: snapshotValue,
    agents: agentsValue,
    repositories,
    allSessions,
    activeSession: null,
    workspaces,
    workflows,
    turns: [],
    userSettingsResponse: optionalValue(userSettingsResponse),
    messagesResponse: null,
  });

  return { task, sessionId: null, initialState, initialTerminals: [] };
}

type TerminalApiResponse = Awaited<ReturnType<typeof fetchTerminals>>[number];

/** Whether a terminal from the API should be hydrated on SSR (skips placeholder and parked terminals). */
function shouldHydrateTerminal(t: TerminalApiResponse): boolean {
  const id = t.id ?? t.terminal_id ?? "";
  if (!id || id === "bottom-panel") return false;
  if (t.state === "parked") return false;
  return true;
}

/** Classifies a terminal as script and/or ordinary based on its kind, id prefix, and seq. */
function classifyTerminal(
  t: TerminalApiResponse,
  id: string,
): { isScript: boolean; isOrdinary: boolean } {
  const isScript = t.kind === "script" || id.startsWith("script-");
  const isOrdinary = t.kind === "ordinary" || (!isScript && t.seq !== undefined);
  return { isScript, isOrdinary };
}

/**
 * Derives the display label for a hydrated terminal: explicit names win, then a
 * numbered "Terminal N" for ordinary terminals, then script/terminal defaults.
 */
function deriveHydratedLabel(
  t: TerminalApiResponse,
  isScript: boolean,
  isOrdinary: boolean,
): string {
  if (t.display_name) return t.display_name;
  if (t.custom_name && t.custom_name !== "") return t.custom_name;
  if (t.label) return t.label;
  if (isOrdinary && t.seq) return translate("common:terminalNumbered", { seq: t.seq });
  return isScript ? translate("common:script") : translate("common:terminal");
}

/** Maps the classification flags to a terminal kind ("ordinary", "script", or undefined). */
function pickTerminalKind(
  isOrdinary: boolean,
  isScript: boolean,
): "ordinary" | "script" | undefined {
  if (isOrdinary) return "ordinary";
  if (isScript) return "script";
  return undefined;
}

/** Maps a terminal API response to the Terminal model used by the session page. */
function hydrateTerminal(t: TerminalApiResponse): Terminal {
  const id = (t.id ?? t.terminal_id ?? "") as string;
  const { isScript, isOrdinary } = classifyTerminal(t, id);
  const kind = pickTerminalKind(isOrdinary, isScript);
  return {
    id,
    type: isScript ? ("script" as const) : ("shell" as const),
    label: deriveHydratedLabel(t, isScript, isOrdinary),
    closable: t.closable ?? true,
    kind,
    seq: t.seq,
    customName: t.custom_name ?? undefined,
    state: t.state,
    ptyStatus: t.pty_status,
  };
}

/**
 * Shared SSR pipeline: fans out optional enrichment requests (workflow snapshot,
 * agents, repositories, workspaces, workflows, turns, user settings, terminals,
 * messages) under one optional-hydration deadline, then assembles the initial
 * state and the hydrated terminals.
 */
async function fetchSessionDataFromTask(
  task: Task,
  sessionId: string,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
  activeSessionResponse: Promise<OptionalHydrationResult<{ session?: TaskSession | null }>>,
  optionalHydration: ReturnType<typeof beginOptionalHydration>,
): Promise<FetchedSessionData> {
  // User shells are env-scoped — look up this session's task_environment_id
  // from the already-fetched session list. Sessions w/o env (legacy) skip
  // the terminal SSR fetch; the boot-time heal pass + WS-driven user_shell.list
  // will populate it once the env mapping lands.
  const sessionEnvId =
    allSessionsResponse.sessions?.find((s) => s.id === sessionId)?.task_environment_id ?? "";

  const results = await Promise.all([
    // The required task and session list were fetched before this optional fan-out.
    optionalHydration.load("workflow snapshot", () =>
      task.workflow_id
        ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
        : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    ),
    optionalHydration.load("agents", () => listAgents({ cache: "no-store" })),
    optionalHydration.load("repositories", () =>
      listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    ),
    optionalHydration.load("workspaces", () => listWorkspaces({ cache: "no-store" })),
    optionalHydration.load("workflows", () =>
      listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }),
    ),
    optionalHydration.load("session turns", () =>
      listSessionTurns(sessionId, { cache: "no-store" }),
    ),
    optionalHydration.load("user settings", () => fetchUserSettings({ cache: "no-store" })),
    optionalHydration.load("terminals", () =>
      sessionEnvId ? fetchTerminals(task.id, sessionEnvId) : Promise.resolve([]),
    ),
    optionalHydration.load("messages", () =>
      listTaskSessionMessages(sessionId, { limit: 50, sort: "desc" }, { cache: "no-store" }),
    ),
    activeSessionResponse,
  ]);
  optionalHydration.complete();
  const [
    snapshot,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    turnsResponse,
    userSettingsResponse,
    terminalsResponse,
    messagesResponse,
    activeSessionResult,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const snapshotValue = optionalValue(snapshot);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;
  const turns = optionalValue(turnsResponse)?.turns;
  const terminals = optionalValue(terminalsResponse) ?? [];
  const messages = optionalValue(messagesResponse);
  const activeSession = optionalValue(activeSessionResult)?.session ?? null;

  const initialTerminals: Terminal[] = terminals.filter(shouldHydrateTerminal).map(hydrateTerminal);

  const initialState = buildSessionPageState({
    task,
    sessionId,
    snapshot: snapshotValue,
    agents: agentsValue,
    repositories,
    allSessions,
    activeSession,
    workspaces,
    workflows,
    turns,
    userSettingsResponse: optionalValue(userSettingsResponse),
    messagesResponse: messages,
  });

  return { task, sessionId, initialState, initialTerminals };
}

/** Returns the repositories seeded for a task's workspace, or [] when absent. */
export function extractInitialRepositories(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  return initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ""] ?? [];
}

/** Returns the scripts seeded for a task's first repository, or [] when absent. */
export function extractInitialScripts(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  const repoId = task?.repositories?.[0]?.repository_id ?? "";
  return initialState?.repositoryScripts?.itemsByRepositoryId?.[repoId] ?? [];
}
