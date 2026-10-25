"use client";

import {
  useKubernetesDiagnostics,
  useKubernetesSessions,
} from "@/hooks/domains/settings/use-kubernetes-settings";
import type { Executor } from "@/lib/types/http";
import { KubernetesConnectionCard } from "./kubernetes-connection-card";
import {
  serializeKubernetesExecutorConfig,
  serializeKubernetesProfileConfig,
  type KubernetesExecutorForm,
  type KubernetesProfileConfigForm,
} from "./kubernetes-config";
import { KubernetesDiagnosticsCard } from "./kubernetes-diagnostics-card";
import { KubernetesSessionsCard } from "./kubernetes-sessions-card";

type KubernetesDiagnosticsState = ReturnType<typeof useKubernetesDiagnostics>;
type KubernetesSessionsState = ReturnType<typeof useKubernetesSessions>;

type KubernetesProfileClusterSectionProps = {
  executor: Executor;
  form: KubernetesProfileConfigForm;
  connectionForm: KubernetesExecutorForm;
  connectionBaseline: KubernetesExecutorForm;
  onConnectionChange: (form: KubernetesExecutorForm) => void;
  canManage: boolean;
  diagnosticsState?: KubernetesDiagnosticsState;
  sessionsState?: KubernetesSessionsState;
};

export function KubernetesProfileClusterSection({
  executor,
  form,
  connectionForm,
  connectionBaseline,
  onConnectionChange,
  canManage,
  diagnosticsState,
  sessionsState,
}: KubernetesProfileClusterSectionProps) {
  const localDiagnostics = useKubernetesDiagnostics();
  const localSessions = useKubernetesSessions(executor.id, sessionsState === undefined);
  const diagnostics = diagnosticsState ?? localDiagnostics;
  const sessions = sessionsState ?? localSessions;

  const runTest = () => {
    void diagnostics
      .run({
        config: serializeKubernetesExecutorConfig(connectionForm),
        profile_config: serializeKubernetesProfileConfig(form),
      })
      .catch(() => undefined);
  };

  return (
    <section className="min-w-0 space-y-6" data-testid="kubernetes-profile-cluster-section">
      <KubernetesConnectionCard
        form={connectionForm}
        baseline={connectionBaseline}
        onChange={onConnectionChange}
        readOnly={!canManage}
      />
      <KubernetesDiagnosticsCard
        testing={diagnostics.testing}
        result={diagnostics.result}
        error={diagnostics.error}
        onTest={runTest}
        canManage={canManage}
        includesProfile
      />
      <KubernetesSessionsCard state={sessions} />
    </section>
  );
}
