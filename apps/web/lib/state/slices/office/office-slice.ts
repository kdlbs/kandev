import type { StateCreator } from "zustand";
import type { OfficeSlice, OfficeSliceState } from "./types";

export const defaultTaskFilters = {
  statuses: [] as string[],
  priorities: [] as string[],
  assigneeIds: [] as string[],
  projectIds: [] as string[],
  search: "",
};

export const defaultOfficeState: OfficeSliceState = {
  office: {
    agentProfiles: [],
    skills: [],
    projects: [],
    approvals: [],
    activity: [],
    costSummary: null,
    budgetPolicies: [],
    routines: [],
    inboxItems: [],
    inboxCount: 0,
    runs: [],
    dashboard: null,
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
  },
};

type ImmerSet = StateCreator<OfficeSlice, [["zustand/immer", never]], [], OfficeSlice>;
type SetFn = Parameters<ImmerSet>[0];

function createAgentActions(set: SetFn) {
  return {
    setOfficeAgentProfiles: (agents: OfficeSlice["office"]["agentProfiles"]) =>
      set((draft) => {
        draft.office.agentProfiles = agents;
      }),
    addOfficeAgentProfile: (agent: OfficeSlice["office"]["agentProfiles"][number]) =>
      set((draft) => {
        draft.office.agentProfiles.push(agent);
      }),
    updateOfficeAgentProfile: (
      id: string,
      patch: Partial<OfficeSlice["office"]["agentProfiles"][number]>,
    ) =>
      set((draft) => {
        const idx = draft.office.agentProfiles.findIndex((a) => a.id === id);
        if (idx >= 0) Object.assign(draft.office.agentProfiles[idx], patch);
      }),
    removeOfficeAgentProfile: (id: string) =>
      set((draft) => {
        draft.office.agentProfiles = draft.office.agentProfiles.filter((a) => a.id !== id);
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

function createProjectActions(set: SetFn) {
  return {
    setProjects: (projects: OfficeSlice["office"]["projects"]) =>
      set((draft) => {
        draft.office.projects = projects;
      }),
    addProject: (project: OfficeSlice["office"]["projects"][number]) =>
      set((draft) => {
        draft.office.projects.push(project);
      }),
    updateProject: (id: string, patch: Partial<OfficeSlice["office"]["projects"][number]>) =>
      set((draft) => {
        const idx = draft.office.projects.findIndex((p) => p.id === id);
        if (idx >= 0) Object.assign(draft.office.projects[idx], patch);
      }),
    removeProject: (id: string) =>
      set((draft) => {
        draft.office.projects = draft.office.projects.filter((p) => p.id !== id);
      }),
  };
}

function createTaskActions(set: SetFn) {
  return {
    setTasks: (tasks: OfficeSlice["office"]["tasks"]["items"]) =>
      set((draft) => {
        draft.office.tasks.items = tasks;
      }),
    appendTasks: (tasks: OfficeSlice["office"]["tasks"]["items"]) =>
      set((draft) => {
        // De-dupe by id so refetch / load-more overlaps don't double-render.
        const existing = new Set(draft.office.tasks.items.map((t) => t.id));
        for (const t of tasks) {
          if (!existing.has(t.id)) {
            draft.office.tasks.items.push(t);
            existing.add(t.id);
          }
        }
      }),
    patchTaskInStore: (
      taskId: string,
      patch: Partial<OfficeSlice["office"]["tasks"]["items"][number]>,
    ) =>
      set((draft) => {
        const idx = draft.office.tasks.items.findIndex((t) => t.id === taskId);
        if (idx >= 0) Object.assign(draft.office.tasks.items[idx], patch);
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
    setInboxItems: (items: OfficeSlice["office"]["inboxItems"]) =>
      set((draft) => {
        draft.office.inboxItems = items;
      }),
    setInboxCount: (count: number) =>
      set((draft) => {
        draft.office.inboxCount = count;
      }),
    setRuns: (runs: OfficeSlice["office"]["runs"]) =>
      set((draft) => {
        draft.office.runs = runs;
      }),
    setDashboard: (data: OfficeSlice["office"]["dashboard"]) =>
      set((draft) => {
        draft.office.dashboard = data;
      }),
    setMeta: (meta: OfficeSlice["office"]["meta"]) =>
      set((draft) => {
        draft.office.meta = meta;
      }),
    setOfficeLoading: (loading: boolean) =>
      set((draft) => {
        draft.office.isLoading = loading;
      }),
    setOfficeRefetchTrigger: (type: string) =>
      set((draft) => {
        draft.office.refetchTrigger = { type, timestamp: Date.now() };
      }),
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
