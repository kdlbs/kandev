import type { GitChangeLayer } from "./slices/session-runtime/types";

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

/** A repeatable request to reveal a file inside an already-open commit detail. */
export type CommitFileNavigationRequest = {
  path: string;
  token: number;
};

/** Exact immutable file identity for a retained turn comparison. */
export type HistoricalTurnDiffTarget = {
  sessionId: string;
  changeSetId: string;
  repositoryChangeId?: string;
  fileChangeId?: string;
  checkoutId?: string;
  path?: string;
  fileKind?: string;
  oldPath?: string | null;
};

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
  | {
      kind: "commit";
      target: CommitDetailTarget;
      fileNavigation?: CommitFileNavigationRequest | null;
    }
  | { kind: "historical"; target: HistoricalTurnDiffTarget };
