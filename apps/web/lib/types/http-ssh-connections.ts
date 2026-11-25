export type SSHIdentity = {
  dependencies?: SSHIdentityDependency[];
  id: string;
  label: string;
  kind: "managed" | "file" | "agent";
  public_key: string;
  fingerprint: string;
  reference?: string;
  generation: string;
  version: number;
};
export type SSHIdentityDependency = {
  kind: "connection" | "executor" | "runtime";
  id: string;
  label?: string;
  connection_id?: string;
  task_id?: string;
};
export type ManagedSSHConnection = {
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  alias?: string;
  identity_id: string;
  fingerprint: string;
  public_key: string;
  bastion_id?: string;
  version: number;
  revision: number;
  verified_route?: string;
  imported_pending?: boolean;
  known_hosts_pending?: boolean;
  repair_reason?: string;
};
export type SSHRoute = {
  latest_enrollment?: SSHEnrollment;
  destination: ManagedSSHConnection;
  bastion?: ManagedSSHConnection;
  stamp: string;
};
export type SSHObservation = {
  receipt: string;
  fingerprint: string;
  public_key: string;
  stamp: string;
};
export type CreateSSHIdentity = {
  request_id: string;
  label: string;
  kind: "generate" | "import" | "file" | "agent";
  private_key?: string;
  passphrase?: string;
  reference?: string;
};
export type SaveSSHConnection = {
  request_id?: string;
  expected_version: number;
  connection: ManagedSSHConnection;
};
export type SSHEnrollment = {
  error_code?: string;
  hop?: "bastion" | "destination";
  request_id: string;
  connection_id: string;
  stamp: string;
  key_fingerprint: string;
  created_at: string;
  state: "started" | "verified" | "failed" | "installed_unverified" | "outcome_unknown";
};
export type EnrollSSHKey = {
  request_id: string;
  expected_stamp: string;
  key_fingerprint: string;
  mode: "password" | "identity";
  password?: string;
  bootstrap_identity_id?: string;
};
export type SSHPreflight = {
  stamp: string;
  ready: boolean;
  checks: { code: string; hop: string }[];
};
