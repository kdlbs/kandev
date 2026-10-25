"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { useSandboxAccess } from "@/hooks/domains/settings/use-sandbox-access";
import { useSandboxBinding } from "@/hooks/domains/settings/use-sandbox-binding";
import { useSandboxExecutorReads } from "@/hooks/domains/settings/use-sandbox-executor-reads";
import { emptyExecutorPolicy, executorPolicyReady } from "@/lib/sandbox-executor";
import type { Executor } from "@/lib/types/http";
import type { SandboxExecutorPolicy } from "@/lib/types/sandbox";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { SettingsSaveCancelledError, useSettingsSaveContributor } from "./settings-save-provider";
import { SandboxChoice } from "./sandbox-choice";
import { SandboxPermissions } from "./sandbox-permissions";
import { SandboxPolicyCreate } from "./sandbox-policy-create";

export function ExecutorSandboxSettings({ executor }: { executor: Executor }) {
  const { t } = useTranslation();
  const access = useSandboxAccess();
  if (!access.enabled || !["local", "worktree"].includes(executor.type)) return null;
  if (!access.canRead) return <p className="text-sm">{t("agents:sandboxOperatorRequired")}</p>;
  return (
    <ExecutorSandboxEditor
      key={`${access.identity}:${executor.id}`}
      executorId={executor.id}
      identity={access.identity}
      canEdit={access.canEdit}
    />
  );
}

type Props = { executorId: string; identity: string; canEdit: boolean };
type Model = ReturnType<typeof useExecutorSandboxEditor>;

function useExecutorSandboxEditor({ executorId, identity, canEdit }: Props) {
  const { t } = useTranslation();
  const binding = useSandboxBinding("executor", executorId);
  const policy = binding.draft?.executor ?? emptyExecutorPolicy("disabled");
  const reads = useSandboxExecutorReads(identity, binding.draft?.executor, canEdit);
  const revision = JSON.stringify(binding.draft);
  const ready =
    canEdit &&
    !binding.loading &&
    !binding.saving &&
    !reads.loading &&
    executorPolicyReady(policy, reads.revisions);
  useSettingsSaveContributor({
    id: `sandbox:executor:${executorId}`,
    order: 15,
    revision,
    isDirty: binding.dirty,
    canSave: ready,
    invalidReason: ready ? undefined : t("agents:sandboxExecutorIncomplete"),
    save: async (submitted) => {
      if (!ready || submitted !== revision) throw new SettingsSaveCancelledError();
      await binding.save();
    },
    discard: binding.discard,
  });
  const update = (patch: Partial<SandboxExecutorPolicy>) =>
    binding.update({ executor: { ...policy, ...patch } });
  return { binding, policy, reads, update, canEdit };
}

function ExecutorSandboxEditor(props: Props) {
  const { t } = useTranslation();
  const model = useExecutorSandboxEditor(props);
  const ceiling = model.reads.revisions.find(
    (item) => item.id === model.policy.ceiling_revision_id,
  );
  return (
    <SettingsCard isDirty={model.binding.dirty} data-testid="executor-sandbox-settings">
      <SettingsCardHeader
        title={t("agents:sandboxHeading")}
        description={t("agents:sandboxExecutorHelp")}
      />
      <CardContent className="space-y-4">
        {(model.binding.error || model.reads.failed) && (
          <p role="alert" className="text-sm text-destructive">
            {t(model.binding.errorMessage ?? "agents:sandboxLoadFailed")}
          </p>
        )}
        {!props.canEdit && <p className="text-sm">{t("agents:sandboxOperatorRequired")}</p>}
        <fieldset
          className="min-w-0 space-y-4"
          disabled={!props.canEdit || model.binding.loading || model.binding.saving}
        >
          <SandboxChoice
            label={t("agents:sandboxExecutorMode")}
            value={model.policy.mode}
            placeholder={t("agents:sandboxExecutorDisabled")}
            choices={[
              { value: "disabled", label: t("agents:sandboxExecutorDisabled") },
              { value: "required", label: t("agents:sandboxExecutorRequired") },
            ]}
            onChange={(mode) =>
              model.binding.update({
                executor: emptyExecutorPolicy(mode as SandboxExecutorPolicy["mode"]),
              })
            }
          />
          {model.policy.mode === "required" && <ExecutorSandboxPolicies model={model} />}
        </fieldset>
        {ceiling && <SandboxPermissions grants={ceiling.policy.grants} />}
      </CardContent>
    </SettingsCard>
  );
}

function ExecutorSandboxPolicies({ model }: { model: Model }) {
  const { t } = useTranslation();
  const { policy, reads, update } = model;
  const available = reads.revisions.filter(
    (item) =>
      item.policy.installation_id === policy.installation_id &&
      item.policy.provider_key === policy.provider_key &&
      !item.revoked,
  );
  const choices = available.map((item) => ({
    value: item.id,
    label: `${item.policy.name} (${item.number})`,
    disabled: false,
  }));
  const selected = (id: string) =>
    choices.some((item) => item.value === id) || !id
      ? choices
      : [...choices, { value: id, label: t("agents:sandboxStoredSelection"), disabled: true }];
  return (
    <>
      <ExecutorSandboxProvider model={model} />
      <SandboxChoice
        label={t("agents:sandboxExecutorCeiling")}
        value={policy.ceiling_revision_id}
        choices={selected(policy.ceiling_revision_id)}
        placeholder={t("agents:sandboxChoosePolicy")}
        onChange={(ceiling_revision_id) => update({ ceiling_revision_id })}
      />
      <fieldset className="space-y-1">
        <legend className="text-sm font-medium">{t("agents:sandboxExecutorEligible")}</legend>
        {available.map((item) => (
          <label key={item.id} className="flex min-h-11 cursor-pointer items-center gap-3 text-sm">
            <Checkbox
              checked={policy.eligible_revision_ids.includes(item.id)}
              onCheckedChange={(checked) =>
                update({
                  eligible_revision_ids:
                    checked === true
                      ? [...new Set([...policy.eligible_revision_ids, item.id])]
                      : policy.eligible_revision_ids.filter((id) => id !== item.id),
                })
              }
            />
            <span className="break-words">
              {item.policy.name} ({item.number})
            </span>
          </label>
        ))}
        <UnavailableExecutorPolicies
          model={model}
          availableIds={available.map((item) => item.id)}
        />
      </fieldset>
      <SandboxChoice
        label={t("agents:sandboxPolicyDefault")}
        value={policy.default_revision_id}
        choices={selected(policy.default_revision_id).map((item) => ({
          ...item,
          disabled: item.disabled || !policy.eligible_revision_ids.includes(item.value),
        }))}
        placeholder={t("agents:sandboxChoosePolicy")}
        onChange={(default_revision_id) => update({ default_revision_id })}
      />
    </>
  );
}

function UnavailableExecutorPolicies({
  model,
  availableIds,
}: {
  model: Model;
  availableIds: string[];
}) {
  const { t } = useTranslation();
  return model.policy.eligible_revision_ids
    .filter((id) => !availableIds.includes(id))
    .map((id) => (
      <label
        key={id}
        className="flex min-h-11 cursor-pointer items-center gap-3 text-sm text-destructive"
      >
        <Checkbox
          checked
          onCheckedChange={(checked) => {
            if (checked === false)
              model.update({
                eligible_revision_ids: model.policy.eligible_revision_ids.filter(
                  (value) => value !== id,
                ),
              });
          }}
        />
        <span>
          {model.reads.revisions.find((item) => item.id === id)?.policy.name ??
            t("agents:sandboxStoredSelection")}
          : {t("agents:sandboxSelectionBlocked")}
        </span>
      </label>
    ));
}

function ExecutorSandboxProvider({ model }: { model: Model }) {
  const { t } = useTranslation();
  const { policy, reads, update } = model;
  const providers = reads.providers.data?.providers ?? [];
  const key = (installation: string, provider: string) => JSON.stringify([installation, provider]);
  const value = policy.installation_id ? key(policy.installation_id, policy.provider_key) : "";
  const choices = providers.map((item) => ({
    value: key(item.installation_id, item.provider_key),
    label: `${item.display_name} (${item.version})`,
  }));
  if (value && !choices.some((item) => item.value === value))
    choices.push({ value, label: t("agents:sandboxProviderUnavailable") });
  return (
    <>
      <SandboxChoice
        label={t("agents:sandboxProvider")}
        value={value}
        choices={choices}
        placeholder={t("agents:sandboxChooseProvider")}
        onChange={(next) => {
          const provider = providers.find(
            (item) => key(item.installation_id, item.provider_key) === next,
          );
          if (provider)
            update({
              installation_id: provider.installation_id,
              provider_key: provider.provider_key,
            });
        }}
      />
      {!providers.length && (
        <p className="text-xs text-muted-foreground">{t("agents:sandboxNoProviders")}</p>
      )}
      <SandboxPolicyCreate
        provider={providers.find(
          (item) =>
            item.installation_id === policy.installation_id &&
            item.provider_key === policy.provider_key,
        )}
        onCreated={() => {
          void reads.catalog.refetch();
        }}
      />
    </>
  );
}
