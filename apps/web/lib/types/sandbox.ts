export type SandboxGrants = {
  filesystem: { path?: string; reference?: string; writable: boolean }[];
  network: { host: string; port: number; protocol: "http" | "https" | "tcp" }[];
  kandev_mcp: boolean;
  environment_keys?: string[];
};

export type SandboxPolicy = {
  id: string;
  workspace_id: string;
  name: string;
  installation_id: string;
  provider_key: string;
  grants: SandboxGrants;
  sources: { kind: string; id: string; digest: string }[] | null;
};

export type SandboxRevision = {
  id: string;
  number: number;
  schema_version: number;
  digest: string;
  policy: SandboxPolicy;
  operator_id: string;
  org_id: string;
  created_at: string;
  revoked: boolean;
};

export type SandboxProvider = {
  installation_id: string;
  provider_key: string;
  display_name: string;
  version: string;
  available: boolean;
};

export type SandboxBindingScope = "executor" | "agent" | "workflow_step" | "task";

export type SandboxExecutorPolicy = {
  mode: "disabled" | "required";
  installation_id: string;
  provider_key: string;
  ceiling_revision_id: string;
  eligible_revision_ids: string[];
  default_revision_id: string;
};

export type SandboxBinding = {
  scope: SandboxBindingScope;
  target_id: string;
  workspace_id: string;
  version: number;
  revision_id: string;
  executor?: SandboxExecutorPolicy;
};

export type SandboxSelectionContext = {
  workspace_id?: string;
  executor_id: string;
  agent_profile_id?: string;
  workflow_step_id?: string;
  task_id?: string;
};

export type SandboxResolved = {
  bindings: Pick<SandboxBinding, "scope" | "target_id" | "version">[];
  revision: SandboxRevision;
  ceiling: SandboxRevision;
  selection_source: SandboxBindingScope;
  executor_id: string;
  binding_version: number;
};

export type SandboxSelectionPreview = {
  resolved: SandboxResolved | null;
  eligible: SandboxRevision[];
  resolution_error?: string;
};

export type SandboxAvailability = { available: boolean; blocker?: string };
