export type DiffSource = "uncommitted" | "committed" | "pr";

export type OpenDiffOptions = {
  source?: DiffSource;
  repositoryName?: string;
};
