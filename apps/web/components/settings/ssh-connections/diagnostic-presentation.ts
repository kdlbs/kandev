export type SSHRemedy = "edit" | "access" | "identity";
export type SSHHop = "bastion" | "destination";
const remedies: Record<string, SSHRemedy> = {
  ssh_target_required: "edit",
  ssh_identity_required: "edit",
  ssh_trust_required: "access",
  ssh_host_key_changed: "access",
  ssh_bastion_not_ready: "access",
  ssh_authentication_rejected: "access",
  ssh_agent_unavailable: "identity",
  ssh_agent_empty: "identity",
  ssh_key_file_unavailable: "identity",
  ssh_managed_key_unavailable: "identity",
};
export function diagnosticIssue(code: string, hop?: string) {
  const known = Object.hasOwn(remedies, code);
  return {
    code: known ? code : "ssh_operation_failed",
    remedy: known ? remedies[code] : undefined,
    hop: hop === "bastion" || hop === "destination" ? hop : undefined,
    technicalCode: /^ssh_[a-z0-9_]{1,80}$/.test(code) ? code : undefined,
  };
}
