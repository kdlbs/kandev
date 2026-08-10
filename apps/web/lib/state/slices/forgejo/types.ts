import type {
  ForgejoActionPreset,
  ForgejoConfig,
  ForgejoIssue,
  ForgejoIssueWatch,
  ForgejoPullRequest,
  ForgejoRepository,
  ForgejoReviewWatch,
} from "@/lib/types/forgejo";

export type ForgejoQueue = {
  issues: { repository: ForgejoRepository; issue: ForgejoIssue }[];
  pull_requests: { repository: ForgejoRepository; pull_request: ForgejoPullRequest }[];
};

type Loadable<T> = { data: T; loading: boolean; loaded: boolean; error: string | null };

export type ForgejoSliceState = {
  forgejoConfig: Record<string, Loadable<ForgejoConfig | null>>;
  forgejoQueue: Record<string, Loadable<ForgejoQueue | null>>;
  forgejoIssueWatches: Record<string, Loadable<ForgejoIssueWatch[]>>;
  forgejoReviewWatches: Record<string, Loadable<ForgejoReviewWatch[]>>;
  forgejoActionPresets: Record<string, Loadable<ForgejoActionPreset[]>>;
  forgejoTaskLinkRevisions: Record<string, Record<string, number>>;
  forgejoWorkspaceDataRevisions: Record<string, number>;
};

export type ForgejoSliceActions = {
  setForgejoConfigState: (
    workspaceId: string,
    data: ForgejoConfig | null,
    error?: string | null,
  ) => void;
  setForgejoConfigLoading: (workspaceId: string, loading: boolean) => void;
  setForgejoQueueState: (
    workspaceId: string,
    data: ForgejoQueue | null,
    error?: string | null,
  ) => void;
  setForgejoQueueLoading: (workspaceId: string, loading: boolean) => void;
  setForgejoIssueWatchesState: (
    workspaceId: string,
    data: ForgejoIssueWatch[],
    error?: string | null,
  ) => void;
  setForgejoIssueWatchesLoading: (workspaceId: string, loading: boolean) => void;
  setForgejoReviewWatchesState: (
    workspaceId: string,
    data: ForgejoReviewWatch[],
    error?: string | null,
  ) => void;
  setForgejoReviewWatchesLoading: (workspaceId: string, loading: boolean) => void;
  setForgejoActionPresetsState: (
    workspaceId: string,
    data: ForgejoActionPreset[],
    error?: string | null,
  ) => void;
  setForgejoActionPresetsLoading: (workspaceId: string, loading: boolean) => void;
  markForgejoTaskLinksUpdated: (workspaceId: string, taskId: string) => void;
  markForgejoWorkspaceDataUpdated: (workspaceId: string) => void;
  resetForgejoWorkspaceState: (workspaceId: string) => void;
};

export type ForgejoSlice = ForgejoSliceState & ForgejoSliceActions;
