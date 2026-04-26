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
    isLoading: false,
  },
};

export const createOrchestrateSlice: StateCreator<
  OrchestrateSlice,
  [["zustand/immer", never]],
  [],
  OrchestrateSlice
> = (set) => ({
  ...defaultOrchestrateState,
  setAgentInstances: (agents) =>
    set((draft) => {
      draft.orchestrate.agentInstances = agents;
    }),
  addAgentInstance: (agent) =>
    set((draft) => {
      draft.orchestrate.agentInstances.push(agent);
    }),
  updateAgentInstance: (id, patch) =>
    set((draft) => {
      const idx = draft.orchestrate.agentInstances.findIndex((a) => a.id === id);
      if (idx >= 0) {
        Object.assign(draft.orchestrate.agentInstances[idx], patch);
      }
    }),
  removeAgentInstance: (id) =>
    set((draft) => {
      draft.orchestrate.agentInstances = draft.orchestrate.agentInstances.filter(
        (a) => a.id !== id,
      );
    }),
  setSkills: (skills) =>
    set((draft) => {
      draft.orchestrate.skills = skills;
    }),
  addSkill: (skill) =>
    set((draft) => {
      draft.orchestrate.skills.push(skill);
    }),
  updateSkill: (id, patch) =>
    set((draft) => {
      const idx = draft.orchestrate.skills.findIndex((s) => s.id === id);
      if (idx >= 0) {
        Object.assign(draft.orchestrate.skills[idx], patch);
      }
    }),
  removeSkill: (id) =>
    set((draft) => {
      draft.orchestrate.skills = draft.orchestrate.skills.filter((s) => s.id !== id);
    }),
  setProjects: (projects) =>
    set((draft) => {
      draft.orchestrate.projects = projects;
    }),
  addProject: (project) =>
    set((draft) => {
      draft.orchestrate.projects.push(project);
    }),
  updateProject: (id, patch) =>
    set((draft) => {
      const idx = draft.orchestrate.projects.findIndex((p) => p.id === id);
      if (idx >= 0) {
        Object.assign(draft.orchestrate.projects[idx], patch);
      }
    }),
  removeProject: (id) =>
    set((draft) => {
      draft.orchestrate.projects = draft.orchestrate.projects.filter((p) => p.id !== id);
    }),
  setApprovals: (approvals) =>
    set((draft) => {
      draft.orchestrate.approvals = approvals;
    }),
  setActivity: (entries) =>
    set((draft) => {
      draft.orchestrate.activity = entries;
    }),
  setCostSummary: (summary) =>
    set((draft) => {
      draft.orchestrate.costSummary = summary;
    }),
  setBudgetPolicies: (policies) =>
    set((draft) => {
      draft.orchestrate.budgetPolicies = policies;
    }),
  setRoutines: (routines) =>
    set((draft) => {
      draft.orchestrate.routines = routines;
    }),
  setInboxItems: (items) =>
    set((draft) => {
      draft.orchestrate.inboxItems = items;
    }),
  setInboxCount: (count) =>
    set((draft) => {
      draft.orchestrate.inboxCount = count;
    }),
  setWakeups: (wakeups) =>
    set((draft) => {
      draft.orchestrate.wakeups = wakeups;
    }),
  setDashboard: (data) =>
    set((draft) => {
      draft.orchestrate.dashboard = data;
    }),
  setIssues: (issues) =>
    set((draft) => {
      draft.orchestrate.issues.items = issues;
    }),
  setIssueFilters: (filters) =>
    set((draft) => {
      Object.assign(draft.orchestrate.issues.filters, filters);
    }),
  setIssueViewMode: (mode) =>
    set((draft) => {
      draft.orchestrate.issues.viewMode = mode;
    }),
  setIssueSortField: (field) =>
    set((draft) => {
      draft.orchestrate.issues.sortField = field;
    }),
  setIssueSortDir: (dir) =>
    set((draft) => {
      draft.orchestrate.issues.sortDir = dir;
    }),
  setIssueGroupBy: (groupBy) =>
    set((draft) => {
      draft.orchestrate.issues.groupBy = groupBy;
    }),
  toggleNesting: () =>
    set((draft) => {
      draft.orchestrate.issues.nestingEnabled = !draft.orchestrate.issues.nestingEnabled;
    }),
  setIssuesLoading: (loading) =>
    set((draft) => {
      draft.orchestrate.issues.isLoading = loading;
    }),
  setOrchestrateLoading: (loading) =>
    set((draft) => {
      draft.orchestrate.isLoading = loading;
    }),
});
