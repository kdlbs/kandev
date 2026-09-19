"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { IconAlertTriangle, IconCheck, IconLoader2, IconX } from "@tabler/icons-react";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { settingsActionClassName } from "./settings-control";
import type {
  KubernetesTestResult,
  KubernetesTestStep,
  KubernetesWarning,
} from "@/lib/types/http-kubernetes";

const STEP_LABEL_KEYS: Record<string, string> = {
  configuration: "executors:kubernetesStepConfiguration",
  discovery: "executors:kubernetesStepDiscovery",
  namespace: "executors:kubernetesStepNamespace",
  rbac: "executors:kubernetesStepRbac",
  "admission.pvc": "executors:kubernetesStepPvcAdmission",
  "storage.existing_claim": "executors:kubernetesStepExistingClaim",
  "admission.pod": "executors:kubernetesStepPodAdmission",
  streaming: "executors:kubernetesStepStreaming",
};

type KubernetesDiagnosticsCardProps = {
  testing: boolean;
  result: KubernetesTestResult | null;
  error: unknown;
  onTest: () => void;
  canManage: boolean;
  includesProfile?: boolean;
};

export function KubernetesDiagnosticsCard({
  testing,
  result,
  error,
  onTest,
  canManage,
  includesProfile = false,
}: KubernetesDiagnosticsCardProps) {
  const { t } = useTranslation();
  return (
    <SettingsCard className="min-w-0 overflow-hidden" data-testid="kubernetes-diagnostics-card">
      <SettingsCardHeader
        title={t("executors:kubernetesDiagnosticsTitle")}
        description={
          includesProfile
            ? t("executors:kubernetesProfileDiagnosticsDescription")
            : t("executors:kubernetesDiagnosticsDescription")
        }
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onTest}
            disabled={testing || !canManage}
            data-testid="kubernetes-test-button"
            className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
          >
            {testing ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {testing ? t("executors:kubernetesTesting") : t("executors:kubernetesTest")}
          </Button>
        }
      />
      <CardContent className="space-y-4">
        {!canManage && (
          <p className="text-sm text-muted-foreground">{t("executors:kubernetesAdminTestOnly")}</p>
        )}
        {Boolean(error) && (
          <p role="alert" className="break-words text-sm text-destructive">
            {error instanceof Error && error.message
              ? error.message
              : t("executors:kubernetesTestRequestFailed")}
          </p>
        )}
        {result && <KubernetesDiagnosticResult result={result} />}
      </CardContent>
    </SettingsCard>
  );
}

function KubernetesDiagnosticResult({ result }: { result: KubernetesTestResult }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4" data-testid="kubernetes-test-result">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <Badge variant={result.success ? "default" : "destructive"}>
          {result.success
            ? t("executors:kubernetesTestPassed")
            : t("executors:kubernetesTestFailed")}
        </Badge>
        {result.server_version && (
          <span className="break-all font-mono text-xs text-muted-foreground">
            {result.server_version}
          </span>
        )}
        {result.namespace && (
          <span className="break-all font-mono text-xs text-muted-foreground">
            {result.namespace}
          </span>
        )}
      </div>
      <ol className="space-y-2">
        {result.steps.map((step, index) => (
          <DiagnosticStep key={`${step.key}:${index}`} step={step} />
        ))}
      </ol>
      {result.warnings.length > 0 && <DiagnosticWarnings warnings={result.warnings} />}
      {result.error && <p className="break-words text-sm text-destructive">{result.error}</p>}
    </div>
  );
}

function DiagnosticStep({ step }: { step: KubernetesTestStep }) {
  const { t } = useTranslation();
  const labelKey = STEP_LABEL_KEYS[step.key];
  return (
    <li
      className="flex min-w-0 items-start gap-3 rounded-md border p-3"
      data-testid={`kubernetes-test-step-${step.key}`}
    >
      {step.success ? (
        <IconCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" />
      ) : (
        <IconX className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
      )}
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
          <span className="font-medium">{labelKey ? t(labelKey) : step.key}</span>
          <span className="font-mono text-xs text-muted-foreground">
            {t("executors:kubernetesDurationMs", { duration: step.duration_ms })}
          </span>
        </div>
        <p className="break-words text-sm text-muted-foreground">{step.detail}</p>
        {step.error && <p className="break-words text-sm text-destructive">{step.error}</p>}
      </div>
    </li>
  );
}

function DiagnosticWarnings({ warnings }: { warnings: KubernetesWarning[] }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
      <div className="flex items-center gap-2 font-medium text-amber-700 dark:text-amber-300">
        <IconAlertTriangle className="h-4 w-4" />
        {t("executors:kubernetesWarnings")}
      </div>
      <ul className="space-y-2">
        {warnings.map((warning, index) => (
          <li key={`${warning.path}:${index}`} className="min-w-0 text-sm">
            <code className="break-all text-xs">{warning.path}</code>
            <p className="break-words text-muted-foreground">{warning.message}</p>
          </li>
        ))}
      </ul>
    </div>
  );
}
