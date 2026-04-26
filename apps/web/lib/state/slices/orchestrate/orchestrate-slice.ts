import type { StateCreator } from "zustand";
import type { OrchestrateSlice, OrchestrateSliceState } from "./types";

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
  setSkills: (skills) =>
    set((draft) => {
      draft.orchestrate.skills = skills;
    }),
  setProjects: (projects) =>
    set((draft) => {
      draft.orchestrate.projects = projects;
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
  setOrchestrateLoading: (loading) =>
    set((draft) => {
      draft.orchestrate.isLoading = loading;
    }),
});
