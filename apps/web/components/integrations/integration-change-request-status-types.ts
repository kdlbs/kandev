export type IntegrationChangeRequestPipelineState = "success" | "failure" | "pending" | "neutral";

export type IntegrationChangeRequestPipelineRow = {
  id: string;
  label: string;
  state: IntegrationChangeRequestPipelineState;
  detail?: string;
  url?: string;
};

export type IntegrationChangeRequestReviewSummary = {
  state: "approved" | "changes_requested" | "pending";
  approved: number;
  required?: number;
  requested?: number;
};

export type IntegrationChangeRequestStatusItem = {
  id: string;
  number: number | string;
  title: string;
  repositoryLabel?: string;
  url?: string;
  state: "open" | "merged" | "closed" | "draft";
  status?: IntegrationChangeRequestPipelineState;
  pipelineRows?: readonly IntegrationChangeRequestPipelineRow[];
  review?: IntegrationChangeRequestReviewSummary;
  unresolvedComments?: number;
  loading?: boolean;
  error?: string | null;
  updatedAt?: number;
  onRefresh?: () => void | Promise<void>;
  onOpenReview: () => void;
  onUnlink?: () => void | Promise<void>;
};

export type IntegrationChangeRequestStatusProps = {
  items: readonly IntegrationChangeRequestStatusItem[];
  surface?: "topbar" | "composer";
};
