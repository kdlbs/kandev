export type MCPExecutionMode = "remote" | "managed_package" | "existing_executable";
export type MCPDefinitionSource = "curated" | "registry" | "custom" | "legacy_import";
export type MCPTransport = "stdio" | "http" | "sse" | "streamable_http";

export type MCPServerConfiguration = {
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  headers?: Record<string, string>;
  url?: string;
  package_type?: string;
  package_name?: string;
  package_version?: string;
  package_registry?: string;
  options?: Record<string, unknown>;
};

export type MCPSecretBinding = {
  input_name: string;
  secret_id: string;
};

export type MCPServerDefinition = {
  id: string;
  workspace_id: string;
  runtime_name: string;
  normalized_runtime_name: string;
  display_name: string;
  description?: string;
  enabled: boolean;
  execution_mode: MCPExecutionMode;
  transport: MCPTransport;
  configuration: MCPServerConfiguration;
  secret_bindings?: MCPSecretBinding[];
  source: MCPDefinitionSource;
  source_identity?: string;
  revision: number;
  selection_impact?: MCPSelectionImpact;
  created_at: string;
  updated_at: string;
};

export type MCPDefinitionInput = {
  runtime_name: string;
  display_name: string;
  description?: string;
  enabled?: boolean;
  execution_mode: MCPExecutionMode;
  transport: MCPTransport;
  configuration: MCPServerConfiguration;
  secret_bindings?: MCPSecretBinding[];
  source?: MCPDefinitionSource;
  source_identity?: string;
};

export type MCPDefinitionPatch = Partial<MCPDefinitionInput> & {
  expected_revision: number;
};

export type MCPSelectionScope = "profile" | "repository" | "task" | "task_session";

export type MCPSelectionOrigin = {
  scope: MCPSelectionScope;
  workspace_id: string;
  owner_id: string;
};

export type MCPSelectionImpact = {
  profile: number;
  repository: number;
  task: number;
  task_session: number;
};

export type MCPSessionApplyState = "applied" | "pending_idle" | "deferred_restart" | "failed";

export type MCPSessionSelectionState = {
  desired_revision: number;
  applied_revision: number;
  apply_state: MCPSessionApplyState;
  failure_code?: string;
  failure_summary?: string;
  attachment_attempt_id?: string;
};

export type MCPSelectionResponse = {
  workspace_id: string;
  scope: MCPSelectionScope;
  owner_id: string;
  definition_ids: string[];
  mcp_state?: MCPSessionSelectionState;
};

export type MCPInheritedSelection = {
  definition: MCPServerDefinition;
  origins: MCPSelectionOrigin[];
};

export type MCPMarketplaceChoice = {
  id: string;
  kind: "package" | "remote" | string;
  registry_type?: string;
  identifier?: string;
  version?: string;
  transport?: string;
  url?: string;
  selectable: boolean;
  unsupported_reason?: string;
};

export type MCPMarketplaceEntry = {
  name: string;
  title?: string;
  description: string;
  version: string;
  websiteUrl?: string;
  status: "active" | "deprecated" | "deleted" | string;
  status_message?: string;
  revision: number;
  updated_at?: string;
  source: "curated" | "registry" | string;
  publisher_supplied: boolean;
  trust_notice: string;
  choices: MCPMarketplaceChoice[];
};

export type MCPMarketplaceSearchResponse = {
  entries: MCPMarketplaceEntry[];
  stale: boolean;
  degraded: boolean;
  last_successful_at?: string;
};

export type MCPMarketplaceInstallInput = {
  identity: string;
  expected_revision: number;
  choice_id: string;
  runtime_name?: string;
  display_name?: string;
  secret_bindings?: MCPSecretBinding[];
};

export type MCPResolutionSummary = {
  definition_id?: string;
  runtime_name?: string;
  origins?: MCPSelectionOrigin[];
};
