"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import type { AgentUpdateStatus } from "@/lib/api";
import { SettingsGroup, SettingsRow } from "./settings-group";
import { settingsActionClassName } from "./settings-control";
import { useRuntimeAutoUpdatePolicy } from "./use-runtime-auto-update-policy";

function RuntimeOutcome({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const outcome = status.last_outcome;
  if (!outcome) return null;
  const values = {
    agent: status.display_name,
    previous: outcome.previous_version,
    version: outcome.target_version,
  };
  let key = "agents:runtimeFailedBody";
  if (outcome.status === "running") key = "agents:runtimeRunning";
  if (outcome.status === "interrupted") key = "agents:runtimeInterruptedBody";
  if (outcome.status === "succeeded") key = "agents:runtimeSucceededBody";
  return (
    <p className="text-sm" role="status">
      {t(key, values)}
    </p>
  );
}

function RuntimeAction({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  if (status.management === "managed" && status.available && status.enabled && canManage) {
    return (
      <Button asChild variant="outline" className={settingsActionClassName("shrink-0")}>
        <a href={`#installed-agent-${status.agent_name}`}>{t("agents:runtimeManage")}</a>
      </Button>
    );
  }
  if (!status.guidance_url) return null;
  return (
    <Button asChild variant="outline" className={settingsActionClassName("shrink-0")}>
      <a href={status.guidance_url} target="_blank" rel="noopener noreferrer">
        {t("agents:runtimeManualGuidance")}
      </a>
    </Button>
  );
}

function RuntimePolicyRow({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  const { draft, setDraft, isDirty } = useRuntimeAutoUpdatePolicy(status);
  const management = status.management;
  const managed = management === "managed";
  const usable = status.available && status.enabled;
  let ownerKey = "agents:runtimeUnsupported";
  if (managed) ownerKey = "agents:runtimeManaged";
  if (management === "manual") ownerKey = "agents:runtimeManual";
  const controlId = `runtime-automatic-${status.agent_name}`;
  return (
    <div
      id={`runtime-update-${status.agent_name}`}
      className="min-w-0 scroll-mt-4 space-y-3 py-4"
      data-testid={`runtime-policy-${status.agent_name}`}
      data-settings-dirty={isDirty}
    >
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 space-y-1">
          <h4 className="text-sm font-semibold">{status.display_name || status.agent_name}</h4>
          <p className="break-all font-mono text-xs text-muted-foreground">
            {t("agents:runtimeVersionSource", { runtime: status.source || status.runtime_id })}
          </p>
          <p className="text-xs text-muted-foreground">{t(ownerKey)}</p>
          {!usable && (
            <p className="text-xs text-muted-foreground">{t("agents:runtimeNotAvailable")}</p>
          )}
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
            <span>
              {t(managed ? "agents:runtimeSelected" : "agents:runtimeObserved", {
                version: status.effective_version || t("agents:runtimeUnknown"),
              })}
            </span>
            <span>
              {t("agents:runtimeLatest", {
                version: status.latest_version || t("agents:runtimeUnknown"),
              })}
            </span>
          </div>
        </div>
        <RuntimeAction status={status} />
      </div>
      {managed ? (
        <SettingsRow
          label={t("agents:runtimeAutomatic")}
          description={t("agents:runtimeAutomaticHelp")}
          controlId={controlId}
          touchTarget="switch"
          isDirty={isDirty}
          control={
            <Switch
              id={controlId}
              checked={draft}
              onCheckedChange={setDraft}
              disabled={!canManage || (!status.auto_update_supported && !draft)}
            />
          }
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          {t(
            management === "manual" ? "agents:runtimeManualHelp" : "agents:runtimeUnsupportedHelp",
          )}
        </p>
      )}
      <RuntimeOutcome status={status} />
    </div>
  );
}

export function AgentRuntimePolicies() {
  const { t } = useTranslation();
  const { statusByAgent } = useAgentRuntimeUpdateStatuses();
  const statuses = Object.values(statusByAgent);
  const active = statuses.filter((s) => s.available && s.enabled);
  const inactive = statuses.filter((s) => !s.available || !s.enabled);
  const row = (s: AgentUpdateStatus) => (
    <RuntimePolicyRow key={`${s.agent_name}:${s.runtime_id}`} status={s} />
  );
  return (
    <SettingsGroup
      id="runtime-updates"
      title={t("agents:runtimeUpdatesTitle")}
      description={t("agents:runtimeUpdatesDescription")}
      collapsible={false}
    >
      <div className="divide-y">{active.map(row)}</div>
      {inactive.length > 0 && (
        <details className="min-w-0">
          <summary className="cursor-pointer py-3 text-sm">
            {t("agents:runtimeOtherRegistrations", { count: inactive.length })}
          </summary>
          <div className="divide-y">{inactive.map(row)}</div>
        </details>
      )}
    </SettingsGroup>
  );
}
