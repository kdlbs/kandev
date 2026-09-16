"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useToolPayloadRetention } from "@/hooks/domains/system/use-tool-payload-retention";
import { useToolPayloadRetentionDraft } from "@/hooks/domains/system/use-tool-payload-retention-draft";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import { ApiError } from "@/lib/api/client";
import {
  ToolPayloadAutomation,
  ToolPayloadBackupReview,
  ToolPayloadRetentionFields,
} from "./tool-payload-retention-fields";
import { ToolPayloadAnalysis, ToolPayloadRetentionResults } from "./tool-payload-retention-results";

function errorKey(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 409) return "system:toolPayload.conflict";
    if (error.status === 403) return "system:toolPayload.adminOnly";
    if (error.status === 501) return "system:toolPayload.unsupported";
    const body = error.body as { code?: string } | null;
    if (
      ["invalid_stored_policy", "invalid_preparation", "preparation_required"].includes(
        body?.code ?? "",
      )
    )
      return "system:toolPayload.preparationFailed";
    if (body?.code === "backup_choice_required") return "system:toolPayload.chooseBackup";
    if (error.status === 400) return "system:toolPayload.invalidAge";
  }
  return "system:toolPayload.operationFailed";
}

function Preparation({
  remote,
  canEdit,
  dirty,
}: {
  remote: ReturnType<typeof useToolPayloadRetention>;
  canEdit: boolean;
  dirty: boolean;
}) {
  const { t } = useTranslation();
  const status = remote.status;
  if (!status || status.preparation.state === "none" || status.preparation.state === "ready")
    return null;
  const failed = status.preparation.state === "failed";
  // The hook owns and renders action errors, including retry failures.
  const act = (action: Promise<unknown>) => {
    void action.catch(() => undefined);
  };
  return (
    <div className="space-y-2 border-t pt-3" data-testid="tool-payload-preparation">
      <p className="text-sm">
        {t(failed ? "system:toolPayload.preparationFailed" : "system:toolPayload.preparing")}
      </p>
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
        {failed && status.preparation.choice && (
          <Button
            variant="outline"
            className={settingsActionClassName("cursor-pointer")}
            disabled={!canEdit || remote.pending || dirty}
            data-testid="tool-payload-retry"
            onClick={() =>
              act(
                remote.save({
                  ...status.policy,
                  enabled: true,
                  backup_choice: status.preparation.choice || undefined,
                }),
              )
            }
          >
            {t("system:toolPayload.retryBackup")}
          </Button>
        )}
        <Button
          variant="outline"
          className={settingsActionClassName("cursor-pointer")}
          disabled={!canEdit || remote.pending}
          data-testid="tool-payload-cancel-preparation"
          onClick={() => act(remote.save({ ...status.policy, enabled: false }))}
        >
          {t("system:toolPayload.cancelPreparation")}
        </Button>
      </div>
    </div>
  );
}

type Remote = ReturnType<typeof useToolPayloadRetention>;
type Model = ReturnType<typeof useToolPayloadRetentionDraft>;
const actionClass = settingsActionClassName("cursor-pointer");
// The domain hook records failures for the inline error summary.
function act(action: Promise<unknown>) {
  void action.catch(() => undefined);
}

function CleanupActions({ remote, model }: { remote: Remote; model: Model }) {
  const { t } = useTranslation();
  const status = remote.status;
  if (!status) return null;
  const operationId =
    remote.acceptedId || (status.operation?.state === "running" ? status.operation.id : undefined);
  const canRun =
    model.canEdit && status.policy.enabled && !model.dirty && !remote.active && !remote.pending;
  return (
    <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
      <Button
        variant="outline"
        className={actionClass}
        data-testid="tool-payload-run"
        disabled={!canRun}
        onClick={() => act(remote.run(status.policy.revision))}
      >
        {t("system:toolPayload.run")}
      </Button>
      {operationId && !remote.preparing && (
        <Button
          variant="outline"
          className={actionClass}
          data-testid="tool-payload-cancel"
          disabled={!model.canEdit || remote.pending}
          onClick={() => act(remote.cancel(operationId))}
        >
          {t("system:toolPayload.cancel")}
        </Button>
      )}
    </div>
  );
}
function RetentionError({ remote }: { remote: Remote }) {
  const { t } = useTranslation();
  if (remote.error == null) return null;
  return (
    <div
      role="alert"
      tabIndex={-1}
      className="space-y-2 break-words text-sm text-destructive"
      data-testid="tool-payload-error"
    >
      <p>{t(errorKey(remote.error))}</p>
      <Button
        variant="outline"
        className={actionClass}
        disabled={remote.pending}
        onClick={() => act(remote.refresh())}
      >
        {t("system:toolPayload.refresh")}
      </Button>
    </div>
  );
}
function RetentionContent({
  remote,
  model,
  admin,
}: {
  remote: Remote;
  model: Model;
  admin: boolean;
}) {
  const { t } = useTranslation();
  const status = remote.status;
  const draft = model.draft;
  if (!status || !draft) return null;
  return (
    <>
      {!status.supported && <p>{t("system:toolPayload.unsupported")}</p>}
      {!admin && <p>{t("system:toolPayload.adminOnly")}</p>}
      <div className="flex flex-col gap-3 md:flex-row md:items-center">
        <ToolPayloadRetentionFields model={model} pending={remote.pending} />
        <Button
          variant="outline"
          className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
          data-testid="tool-payload-analyze"
          disabled={!model.canEdit || model.invalid || remote.active || remote.pending}
          onClick={() => act(remote.analyze(draft.age))}
        >
          {t("system:toolPayload.analyze")}
        </Button>
      </div>
      <p id="tool-payload-age-help" className="text-xs text-muted-foreground">
        {t("system:toolPayload.ageHelp")}
      </p>
      <div role="status" aria-live="polite" className="space-y-3">
        {(remote.pending || remote.acceptedId) && (
          <p className="text-sm">{t("system:toolPayload.pending")}</p>
        )}
        <ToolPayloadAnalysis status={status} age={draft.age} />
      </div>
      <ToolPayloadAutomation model={model} pending={remote.pending} />
      <ToolPayloadBackupReview model={model} pending={remote.pending} />
      <Preparation remote={remote} canEdit={model.canEdit} dirty={model.dirty} />
      <ToolPayloadRetentionResults status={status} />
      <CleanupActions remote={remote} model={model} />
      <p className="text-xs text-muted-foreground">{t("system:toolPayload.compactionHelp")}</p>
    </>
  );
}
export function ToolPayloadRetentionCard() {
  const { t } = useTranslation();
  const remote = useToolPayloadRetention();
  const admin = useIsAdmin();
  const model = useToolPayloadRetentionDraft(remote, admin);
  return (
    <SettingsCard
      discoveryTargetId={SYSTEM_SETTINGS_TARGETS.toolPayloadRetention}
      data-testid="tool-payload-retention-card"
      className="min-w-0 max-w-3xl"
    >
      <SettingsCardHeader
        title={t("system:toolPayload.title")}
        description={t("system:toolPayload.description")}
      />
      <CardContent className="min-w-0 space-y-4">
        {!remote.status && !remote.error && <p role="status">{t("settings:loading")}</p>}
        <RetentionError remote={remote} />
        <RetentionContent remote={remote} model={model} admin={admin} />
      </CardContent>
    </SettingsCard>
  );
}
