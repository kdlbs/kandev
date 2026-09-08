import type { GitChangeLayer } from "@/lib/state/slices/session-runtime/types";

export type DiffSource = "uncommitted" | "committed" | "pr";
export type ChangeLayer = GitChangeLayer;

export type LocalCommitDetailTarget = {
  source: "local";
  sha: string;
  /** Multi-repo local repository subpath, omitted for the workspace root. */
  repo?: string;
};

export type GitHubCommitDetailTarget = {
  source: "github";
  sha: string;
  workspaceId: string;
  owner: string;
  repo: string;
  /** Local display/group identity for the linked repository. */
  repositoryName?: string;
};

export type CommitDetailTarget = LocalCommitDetailTarget | GitHubCommitDetailTarget;

export type OpenDiffOptions = {
  source?: DiffSource;
  repositoryName?: string;
  prKey?: string;
  changeLayer?: ChangeLayer;
};

export type DiffSheetMode =
  | { kind: "all" }
  | {
      kind: "file";
      path: string;
      sourceFilter?: "all" | DiffSource;
      repositoryName?: string;
      prKey?: string;
      changeLayer?: ChangeLayer;
    }
  | { kind: "commit"; target: CommitDetailTarget };
