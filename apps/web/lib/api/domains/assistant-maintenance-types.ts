import type { AssistantOperation } from "./assistant-types";
export type Improvement = {
  id: string;
  workspace_id: string;
  state: "proposed" | "investigating" | "prepared" | "rejected" | "resolved" | "unknown";
  revision: number;
  origin: string;
  operation: string;
  reason: string;
  cause: string;
  policy_version: string;
  profile_id: string;
  incident_count: number;
  task_count: number;
  repair_task_id: string;
  objective_id: string;
  commit_oid: string;
  prepared_at?: string;
};
export type MaintenanceScope = {
  repository_id: string;
  workflow_id: string;
  workflow_step_id: string;
  profile_id: string;
  files: string[];
  actions: Array<"read" | "patch" | "test" | "commit">;
  image: string;
  positive_check: string[];
  negative_check: string[];
};
export type MaintenanceGrant = {
  revision: number;
  binding_version: number;
  scope: MaintenanceScope;
  base_oid: string;
  expires_at: string;
  revoked_at?: string;
};
export type MaintenanceValidation = {
  tree_oid: string;
  grant_revision: number;
  passed: boolean;
  checks: Array<{
    kind: "positive" | "negative";
    exit_code: number;
    output_hash: string;
    summary: string;
  }>;
};
export type MaintenanceFile = { path: string; content: string; sha256: string };
export type MaintenanceArtifact = {
  base_oid: string;
  commit_oid: string;
  tree_oid: string;
  patch: string;
  patch_sha256: string;
  validation?: MaintenanceValidation;
};
export type ImprovementEvidence = {
  id: string;
  task_id: string;
  session_id: string;
  origin: string;
  reason: string;
  cause: string;
  observed_at: string;
};
export type MaintenanceSuccess = {
  id: string;
  task_id: string;
  task_title: string;
  session_id: string;
  source_id: string;
  completed_at: string;
};
export type MaintenanceOption = {
  id: string;
  resource_id: string;
  kind: "repository" | "workflow" | "step" | "profile";
  name: string;
  workflow_id?: string;
};
export type ImprovementDetail = {
  candidate: Improvement;
  grant: MaintenanceGrant | null;
  validation: MaintenanceValidation | null;
  review: { state: "rejected" | "resolved"; created_at: string } | null;
};
export type MaintenanceAction = "prepare" | "patch" | "check" | "commit";
export type MaintenanceRequest = AssistantOperation & {
  action: MaintenanceAction;
  candidate_revision: number;
  grant_revision: number;
  file?: MaintenanceFile;
};
export type MaintenanceGrantRequest = {
  expected_binding_version: number;
  expected_revision: number;
  candidate_revision: number;
  scope: MaintenanceScope;
  expires_at: string;
};
