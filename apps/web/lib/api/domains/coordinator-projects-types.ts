// The Projects member of a coordinator read: present when the stored scope is
// selected, or `{scope: "all"}` while features.coordinatorPhase31 is effective.
// `names` is absent when the project listing failed.
export type CoordinatorWatchProjects =
  | { scope: "all" }
  | {
      scope: "selected";
      repository_ids: string[] | null;
      include_no_repository: boolean;
      names?: string[];
    };

export type ProjectEntryKind = "repository_set" | "repository";
export type ProjectEntry = { kind: ProjectEntryKind; id: string };

// The Projects editor member of a settings read or write.
export type ProjectsConfig = {
  scope: "all" | "selected";
  entries: ProjectEntry[];
  include_no_repository: boolean;
};

export type ProjectsRequest = {
  scope: "all" | "selected";
  entries?: ProjectEntry[];
  include_no_repository?: boolean;
};

export type CoordinatorWatches = {
  scope: "all" | "selected";
  workflow_ids: string[];
  projects?: CoordinatorWatchProjects;
};

export type SettingsProjects = {
  scope: "selected";
  repository_ids: string[] | null;
  include_no_repository: boolean;
};
