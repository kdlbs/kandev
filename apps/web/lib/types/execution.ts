export type CommandSummary = {
  executable: string;
  omitted_arguments: number;
  references: string[];
};

export type WrapperSummary = {
  source: string;
  digest: string;
  command: CommandSummary;
};

export type CommandComposition = {
  wrapper: WrapperSummary | null;
  agent: CommandSummary;
};

export type ExecutionSandboxSnapshot = {
  attempt_id: string;
  generation: number;
  installation_id: string;
  provider_key: string;
  provider_version: string;
  package_digest: string;
  runtime_version: string;
  runtime_digest: string;
  revision_id: string;
  revision_number: number;
  policy_name: string;
  policy_digest: string;
  selection_source: string;
  ceiling_revision_id: string;
  ceiling_digest: string;
};

export type ExecutionSnapshot = CommandComposition & {
  schema_version: 1;
  attempt_id: string;
  session_id: string;
  task_id: string;
  execution_id: string;
  generation: number;
  sequence: number;
  executor_id: string;
  executor_type: string;
  agent_profile_id: string;
  transport: string;
  sandbox: ExecutionSandboxSnapshot | null;
  captured_at: string;
};

export type ExecutionStatus =
  | "not_configured"
  | "configured"
  | "starting"
  | "active"
  | "failed"
  | "unknown"
  | "stopped";

export type ExecutionObservation = {
  facts?: ExecutionFacts;
  schema_version: 1;
  observer_generation: number;
  revision: number;
  session_id: string;
  task_id: string;
  snapshot_sequence: number;
  attempt_id: string;
  generation: number;
  attempt_version: number;
  status: Exclude<ExecutionStatus, "not_configured">;
  provider_health: "available" | "unavailable" | "unknown";
  reason: string;
  mechanism?: string;
  checked_at: string;
  evidence_at?: string;
  valid_until?: string;
};

export type LaunchCheckReceipt = {
  checked_at: string;
  policy_digest: string;
  package_digest: string;
  runtime_digest: string;
  checks: string[];
};
export type RuntimeObservation = {
  source_id: string;
  evidence_digest: string;
  observed_at: string;
  platform: "linux" | "macos";
  landlock_abi?: number;
  execute_enforced?: boolean;
  command_mediation_active?: boolean;
  limitations?: string[];
};
export type ExecutionFacts = {
  schema_version: 1;
  process: { state: "running" | "stopped" | "unknown"; checked_at?: string; valid_until?: string };
  launch_checks: { state: "passed" | "unavailable"; receipt?: LaunchCheckReceipt };
  runtime: {
    state: "reported" | "missing" | "unsupported" | "unavailable";
    observations?: RuntimeObservation[];
  };
  audit: {
    integrity: "unchecked" | "valid" | "invalid";
    finalization: "unknown" | "live" | "final" | "interrupted";
    signer: "unchecked" | "unsigned" | "untrusted" | "trusted" | "invalid";
  };
};
