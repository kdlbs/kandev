export type DiffSource = "uncommitted" | "committed" | "pr";

export type OpenDiffOptions = {
  source?: DiffSource;
  repositoryName?: string;
  prKey?: string;
};

export type DiffSheetMode =
  | { kind: "all" }
  | {
      kind: "file";
      path: string;
      sourceFilter?: "all" | DiffSource;
      repositoryName?: string;
      prKey?: string;
    }
  | { kind: "commit"; sha: string; repo?: string };
