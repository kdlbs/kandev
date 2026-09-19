"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";

// The SSH executor's fallback workspace root, mirroring sshDefaultWorkdir in
// apps/backend/internal/agent/runtime/lifecycle. Shown so the help text can
// name a concrete path even for a profile that never set one.
// i18n-exempt: a remote filesystem path, not user-facing copy.
const DEFAULT_SSH_WORKDIR_ROOT = "~/.kandev";

/** The config value that opts a profile in. Compared, never displayed. */
// i18n-exempt: persisted config value matched by the backend with ===.
export const SSH_RECLAIM_ENABLED_VALUE = "true";
// i18n-exempt: persisted config value.
export const SSH_RECLAIM_DISABLED_VALUE = "false";

/** Only the exact string "true" enables reclamation, matching the backend. */
export function isSSHReclaimEnabled(config?: Record<string, string>): boolean {
  return (config?.ssh_reclaim_task_dir ?? "") === SSH_RECLAIM_ENABLED_VALUE;
}

/** The directory tree the setting governs, as the user would type it. */
export function sshTaskDirRoot(config?: Record<string, string>): string {
  const root = (config?.ssh_workdir_root ?? "").trim() || DEFAULT_SSH_WORKDIR_ROOT;
  return `${root.replace(/\/+$/, "")}/tasks/`;
}

/** The host label to name in the blast-radius copy. */
export function sshReclaimHostLabel(executor: Executor): string {
  const config = executor.config ?? {};
  return (config.ssh_host ?? "").trim() || (config.ssh_host_alias ?? "").trim() || executor.name;
}

export type SSHTaskDirReclamationCardProps = {
  executor: Executor;
  profile: ExecutorProfile;
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
};

export function SSHTaskDirReclamationCard({
  executor,
  profile,
  enabled,
  onEnabledChange,
}: SSHTaskDirReclamationCardProps) {
  const { t } = useTranslation();
  const isDirty = enabled !== isSSHReclaimEnabled(profile.config);
  const host = sshReclaimHostLabel(executor);
  const path = sshTaskDirRoot(profile.config);

  return (
    <SettingsCard isDirty={isDirty} data-testid="ssh-task-dir-reclamation-card">
      <SettingsCardHeader
        title={t("executors:sshReclaimTaskDirTitle")}
        description={t("executors:sshReclaimTaskDirDescription", { host, path })}
      />
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="ssh-reclaim-task-dir">{t("executors:sshReclaimTaskDirLabel")}</Label>
            <p className="text-xs text-muted-foreground">
              {t("executors:sshReclaimTaskDirWarning", { host })}
            </p>
          </div>
          <Switch
            id="ssh-reclaim-task-dir"
            data-testid="ssh-reclaim-task-dir"
            checked={enabled}
            data-settings-dirty={isDirty}
            onCheckedChange={onEnabledChange}
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
