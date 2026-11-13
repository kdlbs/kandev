export type BuiltinMCPPackage =
  | "core"
  | "configuration"
  | "rich-output"
  | "walkthroughs"
  | "diagnostics";
export type BuiltinMCPSurface = "kanban-task" | "office-task" | "configuration";
export type BuiltinMCPManifest = {
  version: 1;
  catalog_version: 1;
  workspace_id: string;
  task_id: string;
  surface: BuiltinMCPSurface;
  workspace_revision: number;
  task_revision: number;
  packages: BuiltinMCPPackage[];
  origins: Partial<Record<BuiltinMCPPackage, "required" | "task" | "workspace">>;
};

export type BuiltinMCPAcceptedBinding = {
  manifest: BuiltinMCPManifest;
  session_id: string;
  execution_id: string;
  attachment_attempt_id: string;
  accepted_at: string;
};

export type BuiltinMCPPolicyChoice = "blocked" | "available" | "default";
export type BuiltinMCPPreference = "inherit" | "on" | "off";
export type BuiltinMCPChoices<T extends string> = Partial<Record<BuiltinMCPPackage, T>>;
export type BuiltinMCPPolicy = {
  workspace_id: string;
  version: 1;
  revision: number;
  packages: BuiltinMCPChoices<BuiltinMCPPolicyChoice>;
};
export type BuiltinMCPPreferences = {
  workspace_id: string;
  task_id: string;
  version: 1;
  revision: number;
  packages: BuiltinMCPChoices<BuiltinMCPPreference>;
};
export type BuiltinMCPDescriptor = {
  id: BuiltinMCPPackage;
  runtime_name: string;
  label_key: string;
  description_key: string;
  surfaces: BuiltinMCPSurface[];
  required_surfaces: BuiltinMCPSurface[] | null;
  default_policy: BuiltinMCPPolicyChoice | "required";
  tools: { name: string; actions: string[] | null }[];
};
export type BuiltinMCPSnapshot = {
  policy: BuiltinMCPPolicy;
  preferences?: BuiltinMCPPreferences;
};
export type BuiltinMCPManagementView = BuiltinMCPSnapshot & {
  catalog: BuiltinMCPDescriptor[];
  desired?: BuiltinMCPManifest;
  surface?: BuiltinMCPSurface | "external" | "automation" | "coordinator" | "managed-conversation";
  can_edit: boolean;
  can_choose_for_task?: boolean;
};
export type BuiltinMCPInitialPreferences = {
  version: 1;
  expected_workspace_policy_revision: number;
  packages: BuiltinMCPChoices<BuiltinMCPPreference>;
};
export type BuiltinMCPCreateDraft = {
  workspaceId: string;
  preferences: BuiltinMCPInitialPreferences;
};
export type BuiltinMCPSessionView = {
  workspace_id: string;
  task_id: string;
  session_id: string;
  execution_id?: string;
  desired?: BuiltinMCPManifest;
  bound?: BuiltinMCPAcceptedBinding;
  binding_state: "unknown" | "accepted" | "legacy";
  history?: import("@/lib/state/slices/session-runtime/types").MCPAttachmentHistory;
  walkthrough_availability: "unknown" | "available" | "unavailable";
};
