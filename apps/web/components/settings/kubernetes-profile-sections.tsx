"use client";

import { KubernetesDiagnosticsCard } from "./kubernetes-diagnostics-card";
import { KubernetesWorkloadCard } from "./kubernetes-workload-card";
import { KubernetesWorkspaceCard } from "./kubernetes-workspace-card";
import {
  serializeKubernetesProfileConfig,
  type KubernetesProfileConfigForm,
} from "./kubernetes-config";
import { useKubernetesDiagnostics } from "@/hooks/domains/settings/use-kubernetes-settings";

type KubernetesProfileSectionsProps = {
  executorConfig: Record<string, string>;
  form: KubernetesProfileConfigForm;
  baseline: KubernetesProfileConfigForm;
  onChange: (form: KubernetesProfileConfigForm) => void;
  canManage: boolean;
};

export function KubernetesProfileSections({
  executorConfig,
  form,
  baseline,
  onChange,
  canManage,
}: KubernetesProfileSectionsProps) {
  const diagnostics = useKubernetesDiagnostics();
  const runTest = () => {
    void diagnostics
      .run({
        config: executorConfig,
        profile_config: serializeKubernetesProfileConfig(form),
      })
      .catch(() => undefined);
  };

  return (
    <>
      <KubernetesWorkloadCard
        form={form}
        baseline={baseline}
        onChange={onChange}
        readOnly={!canManage}
      />
      <KubernetesWorkspaceCard
        form={form}
        baseline={baseline}
        onChange={onChange}
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
    </>
  );
}
