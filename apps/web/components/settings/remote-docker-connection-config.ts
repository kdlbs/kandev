import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { buildSSHExecutorConfig } from "@/app/settings/executors/new/[type]/ssh-config";

// The executor config keys the connection form owns. Every one is rewritten on
// save, so a field the form cleared is removed rather than kept from before.
const SSH_CONNECTION_KEYS = [
  "ssh_host",
  "ssh_host_alias",
  "ssh_port",
  "ssh_user",
  "ssh_identity_source",
  "ssh_identity_file",
  "ssh_proxy_jump",
  "ssh_host_fingerprint",
];

/**
 * Maps a saved Remote Docker connection form onto the executor's config.
 *
 * The connection is serialized exactly as the create flow serializes it, so an
 * edited executor stores the same trimmed values a new one would. Config the
 * form does not own is kept.
 */
export function remoteDockerConnectionConfig(
  existing: Record<string, string>,
  cfg: SSHExecutorConfig,
): Record<string, string> {
  const kept = Object.fromEntries(
    Object.entries(existing).filter(([key]) => !SSH_CONNECTION_KEYS.includes(key)),
  );
  return { ...kept, ...buildSSHExecutorConfig(cfg) };
}
