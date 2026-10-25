"use client";

import { settingsActionClassName } from "./settings-control";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useSandboxBinding } from "@/hooks/domains/settings/use-sandbox-binding";
import {
  useSandboxSelectionModel,
  type SandboxSelectionModel,
} from "@/hooks/domains/settings/use-sandbox-selection-model";
import { useSandboxAccess } from "@/hooks/domains/settings/use-sandbox-access";
import { sandboxMessageKey } from "@/lib/sandbox-messages";
import { SANDBOX_INHERIT as INHERIT, sandboxSelectionChoices } from "@/lib/sandbox-selection";
import { SandboxChoice } from "./sandbox-choice";
import { SandboxPermissions } from "./sandbox-permissions";
import { SettingsSaveCancelledError, useSettingsSaveContributor } from "./settings-save-provider";

export type SandboxSelectionSettingsProps = {
  scope: "agent" | "workflow_step" | "task";
  targetId: string;
  agentProfileId?: string;
  readOnly?: boolean;
};

export function SandboxSelectionSettings(props: SandboxSelectionSettingsProps) {
  const { enabled, identity, canRead, canEdit } = useSandboxAccess();
  if (!enabled || !props.targetId) return null;
  return canRead ? (
    <SandboxSelectionEditor
      key={`${identity}:${props.scope}:${props.targetId}`}
      {...props}
      identity={identity}
      canEdit={canEdit && !props.readOnly}
    />
  ) : (
    <SandboxOperatorNotice />
  );
}

function SandboxOperatorNotice() {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 border-t pt-4">
      <h4 className="text-sm font-medium">{t("agents:sandboxHeading")}</h4>
      <p className="text-xs text-muted-foreground">{t("agents:sandboxOperatorRequired")}</p>
    </div>
  );
}

type EditorProps = SandboxSelectionSettingsProps & { identity: string; canEdit: boolean };
type BindingDraft = ReturnType<typeof useSandboxBinding>;
type EditorModel = SandboxSelectionModel;

function useSandboxSelectionSave(binding: BindingDraft, targetKey: string, canSave: boolean) {
  const { t } = useTranslation();
  const revision = JSON.stringify(binding.draft);
  useSettingsSaveContributor({
    id: `sandbox:${targetKey}`,
    order: 15,
    revision,
    isDirty: binding.dirty,
    canSave,
    invalidReason: !canSave ? t("agents:sandboxChooseEligible") : undefined,
    save: async (submitted) => {
      if (submitted !== revision || !canSave) throw new SettingsSaveCancelledError();
      await binding.save();
    },
    discard: binding.discard,
  });
}

function SandboxSelectionEditor(props: EditorProps) {
  const model = useSandboxSelectionModel({ ...props, canRead: true });
  useSandboxSelectionSave(model.binding, `${props.scope}:${props.targetId}`, model.canSave);
  return <SandboxSelectionForm model={model} scope={props.scope} />;
}

export function SandboxSelectionForm({
  model,
  scope,
  inlineChoices = false,
  disabled = false,
}: {
  model: EditorModel;
  scope: SandboxSelectionSettingsProps["scope"];
  inlineChoices?: boolean;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section
      className="space-y-4 border-t pt-4"
      data-testid={`${scope}-sandbox-settings`}
      data-settings-dirty={model.binding.dirty}
      data-settings-dirty-level="container"
    >
      <div className="space-y-1">
        <h4 className="text-sm font-medium">{t("agents:sandboxHeading")}</h4>
        <p className="text-xs text-muted-foreground">{t("agents:sandboxContextHelp")}</p>
      </div>
      <SandboxEditorStatus model={model} canEdit={model.canEdit} />
      {model.canEdit && (
        <SandboxEditorChoices model={model} inlineChoices={inlineChoices} disabled={disabled} />
      )}
      {model.canEdit && <SandboxAvailabilityCheck model={model} />}
      <SandboxSelectionSummary model={model} />
    </section>
  );
}

function SandboxEditorStatus({ model, canEdit }: { model: EditorModel; canEdit: boolean }) {
  const { t } = useTranslation();
  const { binding, reads } = model;
  const failed =
    binding.error || reads.providers.isError || reads.revision.isError || reads.preview.isError;
  return (
    <>
      {binding.loading && <p className="text-xs">{t("common:loading")}</p>}
      {failed && (
        <p role="alert" className="text-xs text-destructive">
          {t(binding.errorMessage ?? "agents:sandboxLoadFailed")}
        </p>
      )}
      {reads.preview.data?.resolution_error && (
        <p role="alert" className="text-xs text-destructive">
          {t(sandboxMessageKey(reads.preview.data.resolution_error))}
        </p>
      )}
      {!canEdit && (
        <p className="text-xs text-muted-foreground">{t("agents:sandboxOperatorRequired")}</p>
      )}
    </>
  );
}

function SandboxAvailabilityCheck({ model }: { model: EditorModel }) {
  const { t } = useTranslation();
  const { availability } = model.reads;
  const completedKey = availability.data?.available
    ? "agents:sandboxAvailabilityChecked"
    : sandboxMessageKey(availability.data?.blocker);
  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("cursor-pointer")}
        disabled={
          !model.executorId ||
          !model.profileContext.profileId ||
          model.binding.loading ||
          model.binding.dirty ||
          availability.isFetching
        }
        onClick={() => void availability.refetch()}
      >
        {t("agents:sandboxCheckAvailability")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("agents:sandboxAvailabilityHelp")}</p>
      {!model.binding.dirty &&
        (availability.isFetching || availability.data || availability.isError) && (
          <p role="status" className="text-xs">
            {t(availability.isFetching ? "common:loading" : completedKey)}
          </p>
        )}
    </div>
  );
}

function SandboxEditorChoices({
  model,
  inlineChoices,
  disabled,
}: {
  model: EditorModel;
  inlineChoices: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const { binding, reads, eligible, executorId, setExecutorId, executors } = model;
  const choices = sandboxSelectionChoices(
    eligible,
    binding.draft?.revision_id ?? "",
    reads.revision.data?.policy.name ?? t("agents:sandboxStoredSelection"),
    t("agents:sandboxInherit"),
  );
  return (
    <>
      {model.profileContext.needsChoice && (
        <SandboxChoice
          native={inlineChoices}
          label={t("agents:sandboxAgentContext")}
          value={model.profileContext.profileId}
          choices={model.profileContext.profiles.map((item) => ({
            value: item.id,
            label: item.label,
          }))}
          onChange={model.profileContext.setPreviewId}
          placeholder={t("agents:sandboxChooseAgentContext")}
          disabled={disabled || binding.loading}
        />
      )}
      <SandboxChoice
        native={inlineChoices}
        label={t("agents:sandboxExecutorContext")}
        value={executorId}
        onChange={setExecutorId}
        placeholder={t("agents:sandboxChooseExecutor")}
        disabled={disabled || binding.loading || model.executorContextFixed}
        choices={executors
          .filter((item) => item.type === "local" || item.type === "worktree")
          .map((item) => ({ value: item.id, label: item.name }))}
      />
      <SandboxChoice
        native={inlineChoices}
        label={t("agents:sandboxPolicyDefault")}
        value={binding.draft?.revision_id || INHERIT}
        onChange={(value) => binding.update({ revision_id: value === INHERIT ? "" : value })}
        choices={choices}
        placeholder={t("agents:sandboxInherit")}
        disabled={
          disabled ||
          !executorId ||
          !model.profileContext.profileId ||
          binding.loading ||
          reads.preview.isFetching
        }
      />
    </>
  );
}

function SandboxSelectionSummary({ model }: { model: EditorModel }) {
  const { t } = useTranslation();
  const { binding, reads } = model;
  const resolved = reads.preview.data?.resolved;
  const effective = binding.draft?.revision_id ? reads.revision.data : resolved?.revision;
  return (
    <>
      <SandboxProviderSummary
        providers={reads.providers.data?.providers}
        policy={effective?.policy}
      />
      {resolved && (
        <p className="text-xs text-muted-foreground">
          {t("agents:sandboxInheritedSource", {
            source: t(`agents:sandboxSource_${resolved.selection_source}`),
            ceiling: resolved.ceiling.policy.name,
            revision: resolved.ceiling.number,
          })}
        </p>
      )}
      {effective && <SandboxPermissions grants={effective.policy.grants} />}
    </>
  );
}

function SandboxProviderSummary({
  providers,
  policy,
}: {
  providers?: import("@/lib/types/sandbox").SandboxProvider[];
  policy?: import("@/lib/types/sandbox").SandboxPolicy;
}) {
  const { t } = useTranslation();
  if (!providers) return null;
  if (providers.length === 0)
    return <p className="text-xs text-muted-foreground">{t("agents:sandboxNoProviders")}</p>;
  if (!policy) return null;
  const provider = providers.find(
    (item) =>
      item.installation_id === policy.installation_id && item.provider_key === policy.provider_key,
  );
  if (!provider)
    return <p className="text-xs text-destructive">{t("agents:sandboxProviderUnavailable")}</p>;
  return (
    <p className="text-xs">
      {provider.display_name} ({provider.version})
      <span className="ml-2 text-muted-foreground">
        {t(
          provider.available
            ? "agents:sandboxProviderRunning"
            : "agents:sandboxProviderUnavailable",
        )}
      </span>
    </p>
  );
}
