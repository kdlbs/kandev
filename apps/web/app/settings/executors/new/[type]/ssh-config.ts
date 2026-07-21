import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";

/**
 * Maps the SSHConnectionCard's flat config to the snake_case `Config` map
 * the backend persists on the Executor row. Empty optional fields are
 * dropped so the JSON we POST is minimal.
 */
export function buildSSHExecutorConfig(cfg: SSHExecutorConfig): Record<string, string> {
  const out: Record<string, string> = {
    ssh_identity_source: cfg.identity_source,
  };
  if (cfg.host_alias?.trim()) out.ssh_host_alias = cfg.host_alias.trim();
  if (cfg.host?.trim()) out.ssh_host = cfg.host.trim();
  if (cfg.port != null) out.ssh_port = String(cfg.port);
  if (cfg.user?.trim()) out.ssh_user = cfg.user.trim();
  if (cfg.identity_file?.trim()) out.ssh_identity_file = cfg.identity_file.trim();
  if (cfg.proxy_jump?.trim()) out.ssh_proxy_jump = cfg.proxy_jump.trim();
  if (cfg.host_fingerprint?.trim()) out.ssh_host_fingerprint = cfg.host_fingerprint.trim();
  return out;
}

/**
 * Inverse of {@link buildSSHExecutorConfig}: reads the backend `Config` map
 * into the form's flat shape so an existing executor can be edited.
 */
export function parseSSHExecutorConfig(
  name: string,
  config?: Record<string, string>,
): Partial<SSHExecutorConfig> {
  const c = config ?? {};
  const portRaw = c.ssh_port;
  // A missing ssh_port intentionally stays `undefined` so the backend's
  // resolver can apply ~/.ssh/config Port inheritance for the configured
  // alias. Substituting 22 here would silently redirect connections that
  // relied on the alias' Port.
  const port = portRaw ? Number.parseInt(portRaw, 10) : undefined;
  return {
    name,
    host_alias: c.ssh_host_alias ?? "",
    host: c.ssh_host ?? "",
    port: port != null && Number.isFinite(port) ? port : undefined,
    user: c.ssh_user ?? "",
    identity_source: (c.ssh_identity_source as "agent" | "file") || "agent",
    identity_file: c.ssh_identity_file ?? "",
    proxy_jump: c.ssh_proxy_jump ?? "",
    host_fingerprint: c.ssh_host_fingerprint || undefined,
  };
}
