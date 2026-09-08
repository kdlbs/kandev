"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Separator } from "@kandev/ui/separator";
import { IconTrash } from "@tabler/icons-react";
import { KubernetesConnectionCard } from "@/components/settings/kubernetes-connection-card";
import { KubernetesDiagnosticsCard } from "@/components/settings/kubernetes-diagnostics-card";
import { KubernetesReadOnlyNotice } from "@/components/settings/kubernetes-read-only-notice";
import { saveWithKubernetesSessionConfirmation } from "@/components/settings/kubernetes-save-confirmation";
import { KubernetesSessionsCard } from "@/components/settings/kubernetes-sessions-card";
import { useAppStore } from "@/components/state-provider";
import {
  SettingsSaveCancelledError,
  useSettingsSaveContributor,
} from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
import {
  isKubernetesExecutorDirty,
  parseKubernetesExecutorConfig,
  serializeKubernetesExecutorConfig,
  type KubernetesExecutorForm,
} from "@/components/settings/kubernetes-config";
import { kubernetesExecutorInvalidReason } from "@/components/settings/kubernetes-validation";
import {
  useKubernetesAdminAccess,
  useKubernetesDiagnostics,
  useKubernetesExecutorResource,
  useKubernetesSessionImpact,
  useKubernetesSessions,
} from "@/hooks/domains/settings/use-kubernetes-settings";
import { getExecutorIcon, getExecutorLabel } from "@/lib/executor-icons";
import { executorProfileSettingsPath } from "@/lib/settings/executor-settings-routes";
import { SettingsRedirect } from "@/src/settings-route-helpers";

const EXECUTORS_ROUTE = "/settings/executors";
const KubernetesIcon = getExecutorIcon("k8s");

export default function KubernetesExecutorPage({ executorId }: { executorId: string }) {
  const profileId = useAppStore(
    (state) =>
      state.executors.items.find(
        (executor) => executor.id === executorId && executor.type === "k8s",
      )?.profiles?.[0]?.id ?? null,
  );
  if (profileId) {
    return (
      <SettingsRedirect
        to={executorProfileSettingsPath({ id: executorId, type: "k8s" }, profileId)}
      />
    );
  }
  return <KubernetesExecutorRecoveryPage executorId={executorId} />;
}

function KubernetesExecutorRecoveryPage({ executorId }: { executorId: string }) {
  const resource = useKubernetesExecutorResource(executorId);
  const { t } = useTranslation();
  if (resource.loading) return <ExecutorMessage message={t("executors:loadingExecutor")} />;
  if (resource.error || !resource.executor) {
    const message =
      resource.error instanceof Error && resource.error.message
        ? resource.error.message
        : t("executors:executorNotFound");
    return <ExecutorMessage message={message} />;
  }
  if (resource.executor.type !== "k8s") {
    return <ExecutorMessage message={t("executors:kubernetesWrongExecutorType")} />;
  }
  return <KubernetesExecutorView executor={resource.executor} resource={resource} />;
}

function ExecutorMessage({ message }: { message: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="break-words text-sm text-muted-foreground">{message}</p>
        <Button
          className="mt-4 min-h-11 cursor-pointer"
          onClick={() => router.push(EXECUTORS_ROUTE)}
        >
          {t("executors:backToExecutors")}
        </Button>
      </CardContent>
    </Card>
  );
}

type ResourceController = ReturnType<typeof useKubernetesExecutorResource>;
type KubernetesExecutorRecord = NonNullable<ResourceController["executor"]>;

function KubernetesExecutorView({
  executor,
  resource,
}: {
  executor: KubernetesExecutorRecord;
  resource: ResourceController;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const canManage = useKubernetesAdminAccess();
  const diagnostics = useKubernetesDiagnostics();
  const sessions = useKubernetesSessions(executor.id);
  const sessionImpact = useKubernetesSessionImpact(executor.id);
  const initial = parseKubernetesExecutorConfig(executor.name, executor.config);
  const [baseline, setBaseline] = useState<KubernetesExecutorForm>(initial);
  const [form, setForm] = useState<KubernetesExecutorForm>(initial);
  const [error, setError] = useState<unknown>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const config = serializeKubernetesExecutorConfig(form);
  const revision = serializeSettingsRevision({ name: form.name.trim(), config });
  const invalidReason = kubernetesExecutorInvalidReason(form, canManage, t);

  const save = useCallback(async () => {
    setError(null);
    try {
      await saveWithKubernetesSessionConfirmation({
        kind: "connection",
        loadActiveSessionCount: sessionImpact.loadActiveSessionCount,
        save: () => resource.update(form.name.trim(), config),
      });
      setBaseline({ ...form });
    } catch (cause) {
      if (!(cause instanceof SettingsSaveCancelledError)) setError(cause);
      throw cause;
    }
  }, [config, form, resource, sessionImpact.loadActiveSessionCount]);

  const discard = useCallback(() => {
    setForm({ ...baseline });
    diagnostics.clear();
    setError(null);
  }, [baseline, diagnostics]);

  useSettingsSaveContributor({
    id: `kubernetes-executor:${executor.id}`,
    revision,
    isDirty: canManage && isKubernetesExecutorDirty(form, baseline),
    canSave: !invalidReason,
    invalidReason,
    save,
    discard,
  });

  const runTest = () => {
    void diagnostics.run({ config }).catch(() => undefined);
  };
  const remove = async () => {
    setDeleting(true);
    setError(null);
    try {
      await resource.remove();
      runWithNavigationBlockerBypassed(() => router.push(EXECUTORS_ROUTE));
    } catch (cause) {
      setError(cause);
    } finally {
      setDeleting(false);
      setDeleteOpen(false);
    }
  };

  return (
    <div className="min-w-0 space-y-8 overflow-x-clip">
      <KubernetesExecutorHeader
        name={executor.name}
        canManage={canManage}
        onDelete={() => setDeleteOpen(true)}
      />
      {!canManage && <KubernetesReadOnlyNotice />}
      <KubernetesConnectionCard
        form={form}
        baseline={baseline}
        onChange={setForm}
        readOnly={!canManage}
      />
      <KubernetesDiagnosticsCard
        testing={diagnostics.testing}
        result={diagnostics.result}
        error={diagnostics.error}
        onTest={runTest}
        canManage={canManage}
      />
      <KubernetesSessionsCard state={sessions} />
      <KubernetesActionError error={error} />
      <DeleteExecutorDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDelete={() => void remove()}
        deleting={deleting}
      />
    </div>
  );
}

function KubernetesActionError({ error }: { error: unknown }) {
  const { t } = useTranslation();
  if (!error) return null;
  const message =
    error instanceof Error && error.message
      ? error.message
      : t("executors:kubernetesExecutorActionFailed");
  return (
    <p role="alert" className="break-words text-sm text-destructive">
      {message}
    </p>
  );
}

function KubernetesExecutorHeader({
  name,
  canManage,
  onDelete,
}: {
  name: string;
  canManage: boolean;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <KubernetesIcon className="h-5 w-5 text-muted-foreground" />
            <h2 className="min-w-0 break-words text-2xl font-bold">{name}</h2>
            <Badge variant="outline" className="text-[10px]">
              {getExecutorLabel("k8s")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("executors:kubernetesExecutorPageDescription")}
          </p>
        </div>
        <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row">
          <Button
            variant="destructive"
            size="sm"
            onClick={onDelete}
            disabled={!canManage}
            className="min-h-11 cursor-pointer text-sm md:min-h-7 md:text-xs"
          >
            <IconTrash className="mr-1.5 h-4 w-4" />
            {t("executors:deleteExecutor")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => router.push(EXECUTORS_ROUTE)}
            className="min-h-11 cursor-pointer text-sm md:min-h-7 md:text-xs"
          >
            {t("executors:backToExecutors")}
          </Button>
        </div>
      </div>
      <Separator />
    </>
  );
}

function DeleteExecutorDialog({
  open,
  onOpenChange,
  onDelete,
  deleting,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  deleting: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("executors:deleteExecutor")}</DialogTitle>
          <DialogDescription>
            {t("executors:kubernetesDeleteExecutorDescription")}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="min-h-11 cursor-pointer md:min-h-9"
          >
            {t("common:cancel")}
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={deleting}
            className="min-h-11 cursor-pointer md:min-h-9"
          >
            {deleting ? t("executors:deleting") : t("executors:delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
