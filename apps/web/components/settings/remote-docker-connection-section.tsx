"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { remoteDockerConnectionConfig } from "@/components/settings/remote-docker-connection-config";
import { parseSSHExecutorConfig } from "@/app/settings/executors/new/[type]/ssh-config";
import { useSaveExecutorConnection } from "@/hooks/domains/settings/use-save-executor-connection";
import { testRemoteDockerConnection } from "@/lib/api/domains/remote-docker-api";
import type { Executor } from "@/lib/types/http";

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

  const initial = parseSSHExecutorConfig(executor.name, executor.config);
  const buildConfig = useCallback(
    (cfg: SSHExecutorConfig) => remoteDockerConnectionConfig(executor.config ?? {}, cfg),
    [executor.config],
  );
  const handleSave = useSaveExecutorConnection(executor.id, buildConfig, onSaved);

  return (
    <div className="space-y-4" data-testid="remote-docker-connection-section">
      <div
        className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
        data-testid="remote-docker-authority-notice"
      >
        {t("executors:remoteDockerAuthorityNotice")}
      </div>
      <SSHConnectionCard
        // A new pinned fingerprint remounts the card, so it shows what was saved.
        key={`${executor.id}:${executor.config?.ssh_host_fingerprint ?? "none"}`}
        initial={initial}
        onSave={handleSave}
        testConnection={testRemoteDockerConnection}
        coordinatedSaveId={`remote-docker-executor:${executor.id}`}
      />
    </div>
  );
}
