"use client";

import { KubernetesWorkloadCard } from "./kubernetes-workload-card";
import { KubernetesWorkspaceCard } from "./kubernetes-workspace-card";
import type { KubernetesProfileConfigForm } from "./kubernetes-config";

type KubernetesProfileSectionsProps = {
  form: KubernetesProfileConfigForm;
  baseline: KubernetesProfileConfigForm;
  onChange: (form: KubernetesProfileConfigForm) => void;
  canManage: boolean;
};

export function KubernetesProfileSections({
  form,
  baseline,
  onChange,
  canManage,
}: KubernetesProfileSectionsProps) {
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
    </>
  );
}
