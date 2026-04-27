import type { StateCreator } from "zustand";
import type { OrchestrateSlice, OrchestrateSliceState } from "./types";

export const defaultIssueFilters = {
  statuses: [] as string[],
  priorities: [] as string[],
  assigneeIds: [] as string[],
  projectIds: [] as string[],
  search: "",
};

export const defaultOrchestrateState: OrchestrateSliceState = {
  orchestrate: {
    agentInstances: [],
    skills: [],
    projects: [],
    approvals: [],
    activity: [],
    costSummary: null,
    budgetPolicies: [],
    routines: [],
    inboxItems: [],
    inboxCount: 0,
    wakeups: [],
    dashboard: null,
    issues: {
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
  },
};

type ImmerSet = StateCreator<OrchestrateSlice, [["zustand/immer", never]], [], OrchestrateSlice>;
type SetFn = Parameters<ImmerSet>[0];

function createAgentActions(set: SetFn) {
  return {
    setAgentInstances: (agents: OrchestrateSlice["orchestrate"]["agentInstances"]) =>
      set((draft) => {
        draft.orchestrate.agentInstances = agents;
      }),
    addAgentInstance: (agent: OrchestrateSlice["orchestrate"]["agentInstances"][number]) =>
      set((draft) => {
        draft.orchestrate.agentInstances.push(agent);
      }),
    updateAgentInstance: (
      id: string,
      patch: Partial<OrchestrateSlice["orchestrate"]["agentInstances"][number]>,
    ) =>
      set((draft) => {
        const idx = draft.orchestrate.agentInstances.findIndex((a) => a.id === id);
        if (idx >= 0) Object.assign(draft.orchestrate.agentInstances[idx], patch);
      }),
    removeAgentInstance: (id: string) =>
      set((draft) => {
        draft.orchestrate.agentInstances = draft.orchestrate.agentInstances.filter(
          (a) => a.id !== id,
        );
      }),
  };
}

function createSkillActions(set: SetFn) {
  return {
    setSkills: (skills: OrchestrateSlice["orchestrate"]["skills"]) =>
      set((draft) => {
        draft.orchestrate.skills = skills;
      }),
    addSkill: (skill: OrchestrateSlice["orchestrate"]["skills"][number]) =>
      set((draft) => {
        draft.orchestrate.skills.push(skill);
      }),
    updateSkill: (id: string, patch: Partial<OrchestrateSlice["orchestrate"]["skills"][number]>) =>
      set((draft) => {
        const idx = draft.orchestrate.skills.findIndex((s) => s.id === id);
        if (idx >= 0) Object.assign(draft.orchestrate.skills[idx], patch);
      }),
    removeSkill: (id: string) =>
      set((draft) => {
        draft.orchestrate.skills = draft.orchestrate.skills.filter((s) => s.id !== id);
      }),
  };
}

function createProjectActions(set: SetFn) {
  return {
    setProjects: (projects: OrchestrateSlice["orchestrate"]["projects"]) =>
      set((draft) => {
        draft.orchestrate.projects = projects;
      }),
    addProject: (project: OrchestrateSlice["orchestrate"]["projects"][number]) =>
      set((draft) => {
        draft.orchestrate.projects.push(project);
      }),
    updateProject: (
      id: string,
      patch: Partial<OrchestrateSlice["orchestrate"]["projects"][number]>,
    ) =>
      set((draft) => {
        const idx = draft.orchestrate.projects.findIndex((p) => p.id === id);
        if (idx >= 0) Object.assign(draft.orchestrate.projects[idx], patch);
      }),
    removeProject: (id: string) =>
      set((draft) => {
        draft.orchestrate.projects = draft.orchestrate.projects.filter((p) => p.id !== id);
      }),
  };
}

function createIssueActions(set: SetFn) {
  return {
    setIssues: (issues: OrchestrateSlice["orchestrate"]["issues"]["items"]) =>
      set((draft) => {
        draft.orchestrate.issues.items = issues;
      }),
    setIssueFilters: (filters: Partial<OrchestrateSlice["orchestrate"]["issues"]["filters"]>) =>
      set((draft) => {
        Object.assign(draft.orchestrate.issues.filters, filters);
      }),
    setIssueViewMode: (mode: OrchestrateSlice["orchestrate"]["issues"]["viewMode"]) =>
      set((draft) => {
        draft.orchestrate.issues.viewMode = mode;
      }),
    setIssueSortField: (field: OrchestrateSlice["orchestrate"]["issues"]["sortField"]) =>
      set((draft) => {
        draft.orchestrate.issues.sortField = field;
      }),
    setIssueSortDir: (dir: OrchestrateSlice["orchestrate"]["issues"]["sortDir"]) =>
      set((draft) => {
        draft.orchestrate.issues.sortDir = dir;
      }),
    setIssueGroupBy: (groupBy: OrchestrateSlice["orchestrate"]["issues"]["groupBy"]) =>
      set((draft) => {
        draft.orchestrate.issues.groupBy = groupBy;
      }),
    toggleNesting: () =>
      set((draft) => {
        draft.orchestrate.issues.nestingEnabled = !draft.orchestrate.issues.nestingEnabled;
      }),
    setIssuesLoading: (loading: boolean) =>
      set((draft) => {
        draft.orchestrate.issues.isLoading = loading;
      }),
  };
}

function createMiscActions(set: SetFn) {
  return {
    setApprovals: (approvals: OrchestrateSlice["orchestrate"]["approvals"]) =>
      set((draft) => {
        draft.orchestrate.approvals = approvals;
      }),
    setActivity: (entries: OrchestrateSlice["orchestrate"]["activity"]) =>
      set((draft) => {
        draft.orchestrate.activity = entries;
      }),
    setCostSummary: (summary: OrchestrateSlice["orchestrate"]["costSummary"]) =>
      set((draft) => {
        draft.orchestrate.costSummary = summary;
      }),
    setBudgetPolicies: (policies: OrchestrateSlice["orchestrate"]["budgetPolicies"]) =>
      set((draft) => {
        draft.orchestrate.budgetPolicies = policies;
      }),
    setRoutines: (routines: OrchestrateSlice["orchestrate"]["routines"]) =>
      set((draft) => {
        draft.orchestrate.routines = routines;
      }),
    setInboxItems: (items: OrchestrateSlice["orchestrate"]["inboxItems"]) =>
      set((draft) => {
        draft.orchestrate.inboxItems = items;
      }),
    setInboxCount: (count: number) =>
      set((draft) => {
        draft.orchestrate.inboxCount = count;
      }),
    setWakeups: (wakeups: OrchestrateSlice["orchestrate"]["wakeups"]) =>
      set((draft) => {
        draft.orchestrate.wakeups = wakeups;
      }),
    setDashboard: (data: OrchestrateSlice["orchestrate"]["dashboard"]) =>
      set((draft) => {
        draft.orchestrate.dashboard = data;
      }),
    setMeta: (meta: OrchestrateSlice["orchestrate"]["meta"]) =>
      set((draft) => {
        draft.orchestrate.meta = meta;
      }),
    setOrchestrateLoading: (loading: boolean) =>
      set((draft) => {
        draft.orchestrate.isLoading = loading;
      }),
  };
}

export const createOrchestrateSlice: ImmerSet = (set) => ({
  ...defaultOrchestrateState,
  ...createAgentActions(set),
  ...createSkillActions(set),
  ...createProjectActions(set),
  ...createIssueActions(set),
  ...createMiscActions(set),
});
