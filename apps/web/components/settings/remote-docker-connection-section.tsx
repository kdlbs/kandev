"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { testRemoteDockerConnection } from "@/lib/api/domains/remote-docker-api";
import { updateExecutor } from "@/lib/api/domains/settings-api";
import type { Executor } from "@/lib/types/http";

/** Reads the saved connection into the form the shared card renders. */
function connectionFormFromExecutor(executor: Executor): Partial<SSHExecutorConfig> {
  const config = executor.config ?? {};
  return {
    name: executor.name,
    host: config.ssh_host ?? "",
    host_alias: config.ssh_host_alias ?? "",
    port: config.ssh_port ? Number(config.ssh_port) : undefined,
    user: config.ssh_user ?? "",
    identity_source:
      (config.ssh_identity_source as SSHExecutorConfig["identity_source"]) ?? "agent",
    identity_file: config.ssh_identity_file ?? "",
    proxy_jump: config.ssh_proxy_jump ?? "",
    host_fingerprint: config.ssh_host_fingerprint ?? "",
  };
}

/** Maps the form back onto the executor's stored connection config. */
function connectionConfigFromForm(
  executor: Executor,
  cfg: SSHExecutorConfig,
): Record<string, string> {
  return {
    ...(executor.config ?? {}),
    ssh_host: cfg.host ?? "",
    ssh_host_alias: cfg.host_alias ?? "",
    ssh_port: cfg.port ? String(cfg.port) : "",
    ssh_user: cfg.user ?? "",
    ssh_identity_source: cfg.identity_source,
    ssh_identity_file: cfg.identity_file ?? "",
    ssh_proxy_jump: cfg.proxy_jump ?? "",
    ssh_host_fingerprint: cfg.host_fingerprint ?? "",
  };
}

/**
 * Connection settings for a saved Remote Docker executor.
 *
 * A saved profile needs a retest path for the same reason the SSH executor
 * has one: a rotated host key is a hard failure on every later connection, and
 * without a way to retest and re-trust the profile is stuck. The
 * effective-root notice repeats here because this is where the connection is
 * changed, not only where it was first created.
 */
export function RemoteDockerConnectionSection({
  executor,
  onSaved,
}: {
  executor: Executor;
  onSaved?: () => void;
}) {
  const { t } = useTranslation();

  const initial = connectionFormFromExecutor(executor);

  const handleSave = useCallback(
    async (cfg: SSHExecutorConfig) => {
      await updateExecutor(executor.id, {
        name: cfg.name,
        config: connectionConfigFromForm(executor, cfg),
      });
      onSaved?.();
    },
    [executor, onSaved],
  );

  return (
    <div className="space-y-4" data-testid="remote-docker-connection-section">
      <div
        className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
        data-testid="remote-docker-authority-notice"
      >
        {t("executors:remoteDockerAuthorityNotice")}
      </div>
      <SSHConnectionCard
        initial={initial}
        onSave={handleSave}
        testConnection={testRemoteDockerConnection}
        coordinatedSaveId={`remote-docker-executor:${executor.id}`}
      />
    </div>
  );
}
