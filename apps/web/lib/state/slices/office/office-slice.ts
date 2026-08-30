import type { StateCreator } from "zustand";
import type { OfficeSlice, OfficeSliceState } from "./types";
import { normalizeOfficeTask, normalizeTaskStatus } from "@/lib/api/domains/office-task-normalize";

export const defaultTaskFilters = {
  statuses: [] as string[],
  priorities: [] as string[],
  assigneeIds: [] as string[],
  projectIds: [] as string[],
  search: "",
};

export const defaultOfficeState: OfficeSliceState = {
  office: {
    agentProfilesByWorkspaceId: {},
    skills: [],
    projectsByWorkspaceId: {},
    approvals: [],
    activity: [],
    costSummary: null,
    budgetPolicies: [],
    routines: [],
    inboxItemsByWorkspaceId: {},
    inboxCountByWorkspaceId: {},
    runs: [],
    dashboardByWorkspaceId: {},
    tasks: {
      items: [],
      filters: {
        statuses: [],
        priorities: [],
        assigneeIds: [],
        projectIds: [],
        search: "",
      },
      viewMode: "list",
      sortField: "updated",
      sortDir: "desc",
      groupBy: "none",
      nestingEnabled: true,
      isLoading: false,
    },
    meta: null,
    isLoading: false,
    refetchTrigger: null,
    routing: {
      byWorkspace: {},
      knownProviders: [],
      preview: { byWorkspace: {} },
    },
    providerHealth: { byWorkspace: {} },
    runAttempts: { byRunId: {} },
    agentRouting: { byAgentId: {} },
    taskQuorum: { byTaskId: {} },
  },
};

type ImmerSet = StateCreator<OfficeSlice, [["zustand/immer", never]], [], OfficeSlice>;
type SetFn = Parameters<ImmerSet>[0];

type AgentProfiles = OfficeSlice["office"]["agentProfilesByWorkspaceId"][string];

function createAgentActions(set: SetFn) {
  return {
    setOfficeAgentProfiles: (workspaceId: string, agents: AgentProfiles) =>
      set((draft) => {
        draft.office.agentProfilesByWorkspaceId[workspaceId] = agents;
      }),
    addOfficeAgentProfile: (workspaceId: string, agent: AgentProfiles[number]) =>
      set((draft) => {
        const list = draft.office.agentProfilesByWorkspaceId[workspaceId] ?? [];
        list.push(agent);
        draft.office.agentProfilesByWorkspaceId[workspaceId] = list;
      }),
    updateOfficeAgentProfile: (
      workspaceId: string,
      id: string,
      patch: Partial<AgentProfiles[number]>,
    ) =>
      set((draft) => {
        const list = draft.office.agentProfilesByWorkspaceId[workspaceId];
        if (!list) return;
        const idx = list.findIndex((a) => a.id === id);
        if (idx >= 0) Object.assign(list[idx], patch);
      }),
    removeOfficeAgentProfile: (workspaceId: string, id: string) =>
      set((draft) => {
        const list = draft.office.agentProfilesByWorkspaceId[workspaceId];
        if (!list) return;
        draft.office.agentProfilesByWorkspaceId[workspaceId] = list.filter((a) => a.id !== id);
      }),
  };
}

function createSkillActions(set: SetFn) {
  return {
    setSkills: (skills: OfficeSlice["office"]["skills"]) =>
      set((draft) => {
        draft.office.skills = skills;
      }),
    addSkill: (skill: OfficeSlice["office"]["skills"][number]) =>
      set((draft) => {
        draft.office.skills.push(skill);
      }),
    updateSkill: (id: string, patch: Partial<OfficeSlice["office"]["skills"][number]>) =>
      set((draft) => {
        const idx = draft.office.skills.findIndex((s) => s.id === id);
        if (idx >= 0) Object.assign(draft.office.skills[idx], patch);
      }),
    removeSkill: (id: string) =>
      set((draft) => {
        draft.office.skills = draft.office.skills.filter((s) => s.id !== id);
      }),
  };
}

type Projects = OfficeSlice["office"]["projectsByWorkspaceId"][string];

function createProjectActions(set: SetFn) {
  return {
    setProjects: (workspaceId: string, projects: Projects) =>
      set((draft) => {
        draft.office.projectsByWorkspaceId[workspaceId] = projects;
      }),
    addProject: (workspaceId: string, project: Projects[number]) =>
      set((draft) => {
        const list = draft.office.projectsByWorkspaceId[workspaceId] ?? [];
        list.push(project);
        draft.office.projectsByWorkspaceId[workspaceId] = list;
      }),
    updateProject: (workspaceId: string, id: string, patch: Partial<Projects[number]>) =>
      set((draft) => {
        const list = draft.office.projectsByWorkspaceId[workspaceId];
        if (!list) return;
        const idx = list.findIndex((p) => p.id === id);
        if (idx >= 0) Object.assign(list[idx], patch);
      }),
    removeProject: (workspaceId: string, id: string) =>
      set((draft) => {
        const list = draft.office.projectsByWorkspaceId[workspaceId];
        if (!list) return;
        draft.office.projectsByWorkspaceId[workspaceId] = list.filter((p) => p.id !== id);
      }),
  };
}

type StoredTask = OfficeSlice["office"]["tasks"]["items"][number];

// Normalizes a freshly-ingested task's status, keeping the raw pre-
// normalization value on `rawStatus` for the few consumers that need a
// sub-state the canonical union collapses (see OfficeTask.rawStatus).
function normalizeIngestedTask(task: StoredTask): StoredTask {
  return normalizeOfficeTask(task);
}

function createTaskActions(set: SetFn) {
  return {
    setTasks: (tasks: OfficeSlice["office"]["tasks"]["items"]) =>
      set((draft) => {
        draft.office.tasks.items = tasks.map(normalizeIngestedTask);
      }),
    appendTasks: (tasks: OfficeSlice["office"]["tasks"]["items"]) =>
      set((draft) => {
        // De-dupe by id so refetch / load-more overlaps don't double-render.
        const existing = new Set(draft.office.tasks.items.map((t) => t.id));
        for (const t of tasks) {
          if (!existing.has(t.id)) {
            draft.office.tasks.items.push(normalizeIngestedTask(t));
            existing.add(t.id);
          }
        }
      }),
    patchTaskInStore: (taskId: string, patch: Partial<StoredTask>) =>
      set((draft) => {
        const idx = draft.office.tasks.items.findIndex((t) => t.id === taskId);
        if (idx < 0) return;
        if (patch.status === undefined) {
          Object.assign(draft.office.tasks.items[idx], patch);
          return;
        }
        // `patch.rawStatus` is only already set when the caller is restoring
        // a full prior snapshot (see useOptimisticTaskMutation's rollback) —
        // preserve it rather than re-deriving from the (already-canonical)
        // snapshot status.
        Object.assign(draft.office.tasks.items[idx], {
          ...patch,
          rawStatus: patch.rawStatus ?? patch.status,
          status: normalizeTaskStatus(patch.status),
        });
      }),
    setTaskFilters: (filters: Partial<OfficeSlice["office"]["tasks"]["filters"]>) =>
      set((draft) => {
        Object.assign(draft.office.tasks.filters, filters);
      }),
    setTaskViewMode: (mode: OfficeSlice["office"]["tasks"]["viewMode"]) =>
      set((draft) => {
        draft.office.tasks.viewMode = mode;
      }),
    setTaskSortField: (field: OfficeSlice["office"]["tasks"]["sortField"]) =>
      set((draft) => {
        draft.office.tasks.sortField = field;
      }),
    setTaskSortDir: (dir: OfficeSlice["office"]["tasks"]["sortDir"]) =>
      set((draft) => {
        draft.office.tasks.sortDir = dir;
      }),
    setTaskGroupBy: (groupBy: OfficeSlice["office"]["tasks"]["groupBy"]) =>
      set((draft) => {
        draft.office.tasks.groupBy = groupBy;
      }),
    toggleNesting: () =>
      set((draft) => {
        draft.office.tasks.nestingEnabled = !draft.office.tasks.nestingEnabled;
      }),
    setTasksLoading: (loading: boolean) =>
      set((draft) => {
        draft.office.tasks.isLoading = loading;
      }),
  };
}

function createMiscActions(set: SetFn) {
  // Per-store closure state for batching setOfficeRefetchTrigger calls; see
  // that action below.
  let pendingRefetchTypes: string[] = [];
  let refetchFlushScheduled = false;

  return {
    setApprovals: (approvals: OfficeSlice["office"]["approvals"]) =>
      set((draft) => {
        draft.office.approvals = approvals;
      }),
    setActivity: (entries: OfficeSlice["office"]["activity"]) =>
      set((draft) => {
        draft.office.activity = entries;
      }),
    setCostSummary: (summary: OfficeSlice["office"]["costSummary"]) =>
      set((draft) => {
        draft.office.costSummary = summary;
      }),
    setBudgetPolicies: (policies: OfficeSlice["office"]["budgetPolicies"]) =>
      set((draft) => {
        draft.office.budgetPolicies = policies;
      }),
    setRoutines: (routines: OfficeSlice["office"]["routines"]) =>
      set((draft) => {
        draft.office.routines = routines;
      }),
    setInboxItems: (
      workspaceId: string,
      items: OfficeSlice["office"]["inboxItemsByWorkspaceId"][string],
    ) =>
      set((draft) => {
        draft.office.inboxItemsByWorkspaceId[workspaceId] = items;
      }),
    setInboxCount: (workspaceId: string, count: number) =>
      set((draft) => {
        draft.office.inboxCountByWorkspaceId[workspaceId] = count;
      }),
    setRuns: (runs: OfficeSlice["office"]["runs"]) =>
      set((draft) => {
        draft.office.runs = runs;
      }),
    setDashboard: (
      workspaceId: string,
      data: OfficeSlice["office"]["dashboardByWorkspaceId"][string],
    ) =>
      set((draft) => {
        draft.office.dashboardByWorkspaceId[workspaceId] = data;
      }),
    setMeta: (meta: OfficeSlice["office"]["meta"]) =>
      set((draft) => {
        draft.office.meta = meta;
      }),
    setOfficeLoading: (loading: boolean) =>
      set((draft) => {
        draft.office.isLoading = loading;
      }),
    // Same-tick calls (e.g. `task:<id>` immediately followed by `dashboard`
    // for one WS event) are buffered and flushed as a single trigger object
    // in a microtask. Without this, React's automatic batching of the
    // resulting store updates only ever lets a listener observe the *last*
    // write in the tick — any earlier type in the same burst is silently
    // dropped for a listener whose effect hasn't run in between the two
    // `set()` calls.
    setOfficeRefetchTrigger: (type: string) => {
      if (!pendingRefetchTypes.includes(type)) {
        pendingRefetchTypes.push(type);
      }
      if (refetchFlushScheduled) return;
      refetchFlushScheduled = true;
      queueMicrotask(() => {
        const types = pendingRefetchTypes;
        pendingRefetchTypes = [];
        refetchFlushScheduled = false;
        set((draft) => {
          draft.office.refetchTrigger = { types, timestamp: Date.now() };
        });
      });
    },
  };
}

function createRoutingActions(set: SetFn) {
  return {
    setWorkspaceRouting: (
      workspaceId: string,
      cfg: OfficeSlice["office"]["routing"]["byWorkspace"][string],
    ) =>
      set((draft) => {
        draft.office.routing.byWorkspace[workspaceId] = cfg;
      }),
    setKnownProviders: (providers: string[]) =>
      set((draft) => {
        draft.office.routing.knownProviders = providers;
      }),
    setRoutingPreview: (
      workspaceId: string,
      agents: NonNullable<OfficeSlice["office"]["routing"]["preview"]["byWorkspace"][string]>,
    ) =>
      set((draft) => {
        draft.office.routing.preview.byWorkspace[workspaceId] = agents;
      }),
    setProviderHealth: (
      workspaceId: string,
      health: OfficeSlice["office"]["providerHealth"]["byWorkspace"][string],
    ) =>
      set((draft) => {
        draft.office.providerHealth.byWorkspace[workspaceId] = health;
      }),
    upsertProviderHealth: (
      workspaceId: string,
      row: OfficeSlice["office"]["providerHealth"]["byWorkspace"][string][number],
    ) =>
      set((draft) => {
        const list = draft.office.providerHealth.byWorkspace[workspaceId] ?? [];
        const idx = list.findIndex(
          (r) =>
            r.provider_id === row.provider_id &&
            r.scope === row.scope &&
            r.scope_value === row.scope_value,
        );
        if (idx >= 0) list[idx] = row;
        else list.push(row);
        draft.office.providerHealth.byWorkspace[workspaceId] = list;
      }),
    setRunAttempts: (
      runId: string,
      attempts: OfficeSlice["office"]["runAttempts"]["byRunId"][string],
    ) =>
      set((draft) => {
        draft.office.runAttempts.byRunId[runId] = attempts;
      }),
    appendRunAttempt: (
      runId: string,
      attempt: OfficeSlice["office"]["runAttempts"]["byRunId"][string][number],
    ) =>
      set((draft) => {
        const list = draft.office.runAttempts.byRunId[runId] ?? [];
        const idx = list.findIndex((a) => a.seq === attempt.seq);
        if (idx >= 0) list[idx] = attempt;
        else list.push(attempt);
        draft.office.runAttempts.byRunId[runId] = list;
      }),
    setAgentRouting: (
      agentId: string,
      data: OfficeSlice["office"]["agentRouting"]["byAgentId"][string],
    ) =>
      set((draft) => {
        draft.office.agentRouting.byAgentId[agentId] = data;
      }),
    setTaskQuorum: (
      taskId: string,
      quorum: OfficeSlice["office"]["taskQuorum"]["byTaskId"][string],
    ) =>
      set((draft) => {
        draft.office.taskQuorum.byTaskId[taskId] = quorum;
      }),
  };
}

export const createOfficeSlice: ImmerSet = (set) => ({
  ...defaultOfficeState,
  ...createAgentActions(set),
  ...createSkillActions(set),
  ...createProjectActions(set),
  ...createTaskActions(set),
  ...createMiscActions(set),
  ...createRoutingActions(set),
});
