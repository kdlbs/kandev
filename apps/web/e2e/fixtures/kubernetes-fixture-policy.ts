export type FixtureResource = "cluster" | "image";

export type WorkloadRBACRule = {
  apiGroups: string[];
  resources: string[];
  verbs: string[];
};

export type WorkloadRBACProbe = {
  allowed: boolean;
  resource: string;
  verb: string;
};

export const WORKLOAD_RBAC_RULES: WorkloadRBACRule[] = [
  {
    apiGroups: [""],
    resources: ["pods"],
    verbs: ["get", "create", "delete", "watch"],
  },
  {
    apiGroups: [""],
    resources: ["pods/exec", "pods/portforward"],
    verbs: ["get", "create"],
  },
  {
    apiGroups: [""],
    resources: ["persistentvolumeclaims"],
    verbs: ["get", "create", "delete"],
  },
];

export const WORKLOAD_RBAC_PROBES: WorkloadRBACProbe[] = [
  { allowed: true, resource: "pods", verb: "get" },
  { allowed: true, resource: "pods", verb: "create" },
  { allowed: true, resource: "pods", verb: "delete" },
  { allowed: true, resource: "pods", verb: "watch" },
  { allowed: true, resource: "pods/exec", verb: "get" },
  { allowed: true, resource: "pods/exec", verb: "create" },
  { allowed: true, resource: "pods/portforward", verb: "get" },
  { allowed: true, resource: "pods/portforward", verb: "create" },
  { allowed: true, resource: "persistentvolumeclaims", verb: "get" },
  { allowed: true, resource: "persistentvolumeclaims", verb: "create" },
  { allowed: true, resource: "persistentvolumeclaims", verb: "delete" },
  { allowed: false, resource: "pods", verb: "list" },
  { allowed: false, resource: "pods", verb: "patch" },
  { allowed: false, resource: "persistentvolumeclaims", verb: "list" },
  { allowed: false, resource: "persistentvolumeclaims", verb: "watch" },
  { allowed: false, resource: "persistentvolumeclaims", verb: "patch" },
  { allowed: false, resource: "events", verb: "get" },
  { allowed: false, resource: "events", verb: "list" },
  { allowed: false, resource: "events", verb: "create" },
];

export function renderWorkloadRBACRules(): string {
  return WORKLOAD_RBAC_RULES.map(
    (rule) => `  - apiGroups: ${JSON.stringify(rule.apiGroups)}
    resources: ${JSON.stringify(rule.resources)}
    verbs: ${JSON.stringify(rule.verbs)}`,
  ).join("\n");
}

export function assertRuntimeImageTagAvailable(tag: string, exists: boolean): void {
  if (exists) {
    throw new Error(`refusing to overwrite existing Docker image ${tag}`);
  }
}

export function redactKubernetesDiagnosticText(text: string, credential: string): string {
  if (!credential) return text;
  return text.split(credential).join("[REDACTED KUBERNETES CREDENTIAL]");
}

export class FixtureResourceOwnership {
  private readonly owned = new Set<FixtureResource>();

  acquire(resource: FixtureResource, create: () => void): void {
    this.owned.add(resource);
    create();
  }

  owns(resource: FixtureResource): boolean {
    return this.owned.has(resource);
  }

  release(resource: FixtureResource): void {
    this.owned.delete(resource);
  }
}
