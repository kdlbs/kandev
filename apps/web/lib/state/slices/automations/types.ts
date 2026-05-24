import type { Automation, AutomationRun } from "@/lib/types/automation";

export type AutomationsState = {
  items: Automation[];
  loaded: boolean;
  loading: boolean;
};

export type AutomationRunsState = {
  byAutomationId: Record<string, AutomationRun[]>;
  loading: Record<string, boolean>;
};

export type AutomationsSliceState = {
  automations: AutomationsState;
  automationRuns: AutomationRunsState;
};

export type AutomationsSliceActions = {
  setAutomations: (items: Automation[]) => void;
  setAutomationsLoading: (loading: boolean) => void;
  addAutomation: (automation: Automation) => void;
  updateAutomation: (automation: Automation) => void;
  removeAutomation: (id: string) => void;
  setAutomationRuns: (automationId: string, runs: AutomationRun[]) => void;
  setAutomationRunsLoading: (automationId: string, loading: boolean) => void;
};

export type AutomationsSlice = AutomationsSliceState & AutomationsSliceActions;
