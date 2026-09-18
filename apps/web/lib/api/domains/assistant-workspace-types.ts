export type WorkspaceExportKind =
  | "directory"
  | "task_summary"
  | "task_result"
  | "task_input"
  | "handoff";
export type WorkspaceGrantScope = {
  operations: ("observe" | "coordinate")[];
  context_exports: WorkspaceExportKind[];
};
export type WorkspaceReceiver = {
  profile_id: string;
  profile_name: string;
  profile_revision: string;
  authority_revision: string;
};
export type WorkspaceGrant = {
  id: string;
  workspace_id: string;
  workspace_name: string;
  binding_version: number;
  revision: number;
  receiver_profile_id: string;
  receiver_profile_revision: string;
  authority_revision: string;
  scope: WorkspaceGrantScope;
  active: boolean;
  reason?: string;
  revoked_at?: string;
};
export type WorkspaceGrantRequest = {
  expected_binding_version: number;
  expected_revision: number;
  receiver: WorkspaceReceiver;
  scope: WorkspaceGrantScope;
};
export type WorkspaceExportReceipt = {
  id: string;
  workspace_id: string;
  receiver_profile_id: string;
  kind: WorkspaceExportKind | "workspace_link";
  grant_revision: number;
  updated_at: string;
};
export type WorkspaceGrantEvent = {
  id: string;
  revision: number;
  action: "granted" | "revoked" | "forgotten";
  snapshot: WorkspaceGrant;
  created_at: string;
};
