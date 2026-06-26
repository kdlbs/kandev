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
    tasks: {
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
    },
  },
};

type ImmerSet = StateCreator<OfficeSlice, [["zustand/immer", never]], [], OfficeSlice>;
type SetFn = Parameters<ImmerSet>[0];

function createTaskActions(set: SetFn) {
  return {
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
  };
}

export const createOfficeSlice: ImmerSet = (set) => ({
  ...defaultOfficeState,
  ...createTaskActions(set),
});
