"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useArchivedDataRetention } from "@/hooks/domains/system/use-archived-data-retention";
import { useArchivedDataRetentionDraft } from "@/hooks/domains/system/use-archived-data-retention-draft";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import { ApiError } from "@/lib/api/client";
import {
  ArchivedDataAutomation,
  ArchivedDataBackupReview,
  ArchivedDataRetentionFields,
} from "./archived-data-retention-fields";
import {
  ArchivedDataAnalysis,
  ArchivedDataRetentionResults,
} from "./archived-data-retention-results";

type Remote = ReturnType<typeof useArchivedDataRetention>;
type Model = ReturnType<typeof useArchivedDataRetentionDraft>;
const actionClass = settingsActionClassName("cursor-pointer");

function errorKey(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 409) return "system:archivedData.conflict";
    if (error.status === 403) return "system:archivedData.adminOnly";
    if (error.status === 501) return "system:archivedData.unsupported";
    const body = error.body as { code?: string } | null;
    if (
      ["invalid_stored_policy", "invalid_preparation", "preparation_required"].includes(
        body?.code ?? "",
      )
    )
      return "system:archivedData.preparationFailed";
    if (body?.code === "backup_choice_required") return "system:archivedData.chooseBackup";
    if (body?.code === "invalid_age") return "system:archivedData.invalidDays";
    if (error.status === 400) return "system:archivedData.invalidDays";
  }
  return "system:archivedData.failed";
}

// The domain hook records failures for the inline error summary.
function act(action: Promise<unknown>) {
  void action.catch(() => undefined);
}

function Preparation({ remote, model }: { remote: Remote; model: Model }) {
  const { t } = useTranslation();
  const status = remote.status;
  if (!status || status.preparation.state === "none" || status.preparation.state === "ready")
    return null;
  const failed = status.preparation.state === "failed";
  return (
    <div className="space-y-2 border-t pt-3" data-testid="archived-data-preparation">
      <p className="text-sm">
        {t(failed ? "system:archivedData.preparationFailed" : "system:archivedData.preparing")}
      </p>
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
        {failed && status.preparation.choice && (
          <Button
            variant="outline"
            className={actionClass}
            disabled={!model.canEdit || remote.pending || model.dirty}
            data-testid="archived-data-retry"
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
            {t("system:archivedData.retryBackup")}
          </Button>
        )}
        <Button
          variant="outline"
          className={actionClass}
          disabled={!model.canEdit || remote.pending}
          data-testid="archived-data-cancel-preparation"
          onClick={() => act(remote.save({ ...status.policy, enabled: false }))}
        >
          {t("system:archivedData.cancelPreparation")}
        </Button>
      </div>
    </div>
  );
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
        data-testid="archived-data-run"
        disabled={!canRun}
        onClick={() => act(remote.run(status.policy.revision))}
      >
        {t("system:archivedData.run")}
      </Button>
      {operationId && !remote.preparing && (
        <Button
          variant="outline"
          className={actionClass}
          data-testid="archived-data-cancel"
          disabled={!model.canEdit || remote.pending}
          onClick={() => act(remote.cancel(operationId))}
        >
          {t("system:archivedData.cancel")}
        </Button>
      )}
    </div>
  );
}

function RetentionError({ remote }: { remote: Remote }) {
  const { t } = useTranslation();
  if (remote.statusError == null && remote.actionError == null) return null;
  return (
    <div
      role="alert"
      tabIndex={-1}
      className="space-y-2 break-words text-sm text-destructive"
      data-testid="archived-data-error"
    >
      {remote.statusError != null && <p>{t("system:archivedData.statusUnavailable")}</p>}
      {remote.actionError != null && <p>{t(errorKey(remote.actionError))}</p>}
      <Button
        variant="outline"
        className={actionClass}
        disabled={remote.pending}
        onClick={() => act(remote.refresh())}
      >
        {t("system:archivedData.refresh")}
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
      {!status.supported && <p>{t("system:archivedData.unsupported")}</p>}
      {!admin && <p>{t("system:archivedData.adminOnly")}</p>}
      <ArchivedDataRetentionFields model={model} pending={remote.pending} />
      <div className="flex flex-col gap-2 md:flex-row">
        <Button
          variant="outline"
          className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
          data-testid="archived-data-analyze"
          disabled={!model.canEdit || model.invalid || remote.active || remote.pending}
          onClick={() => act(remote.analyze(draft))}
        >
          {t("system:archivedData.analyze")}
        </Button>
      </div>
      <div role="status" aria-live="polite" className="space-y-3">
        {(remote.pending || remote.acceptedId) && (
          <p className="text-sm">{t("system:archivedData.pending")}</p>
        )}
        <ArchivedDataAnalysis status={status} />
      </div>
      <ArchivedDataAutomation model={model} pending={remote.pending} />
      <ArchivedDataBackupReview model={model} pending={remote.pending} />
      <Preparation remote={remote} model={model} />
      <ArchivedDataRetentionResults status={status} />
      <CleanupActions remote={remote} model={model} />
      <p className="text-xs text-muted-foreground">{t("system:archivedData.evidenceHelp")}</p>
    </>
  );
}

export function ArchivedDataRetentionCard() {
  const { t } = useTranslation();
  const remote = useArchivedDataRetention();
  const admin = useIsAdmin();
  const model = useArchivedDataRetentionDraft(remote, admin);
  return (
    <SettingsCard
      discoveryTargetId={SYSTEM_SETTINGS_TARGETS.archivedDataRetention}
      data-testid="archived-data-retention-card"
      className="min-w-0"
    >
      <SettingsCardHeader
        title={t("system:archivedData.title")}
        description={t("system:archivedData.description")}
      />
      <CardContent className="min-w-0 space-y-4">
        {!remote.status && !remote.error && <p role="status">{t("settings:loading")}</p>}
        <RetentionError remote={remote} />
        <RetentionContent remote={remote} model={model} admin={admin} />
      </CardContent>
    </SettingsCard>
  );
}
