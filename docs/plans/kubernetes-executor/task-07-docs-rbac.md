---
id: "07-docs-rbac"
title: "Kubernetes docs and RBAC"
status: completed
wave: 3
depends_on: ["02-lifecycle", "03-service-api", "05-frontend-settings"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 07: Kubernetes docs and RBAC

- **Acceptance:** public executor/Kubernetes/feature-status/security docs match
  shipped auth, template ownership, image, storage, recovery, cleanup, version,
  and trust boundaries.
- **Acceptance:** an opt-in namespaced ServiceAccount/Role/RoleBinding example
  grants only the required executor operations and is not silently applied by
  the existing safe deployment path.
- **Acceptance:** relevant AGENTS guidance and public-doc validation remain current.
- **Verification:** `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check`
- **Files likely touched:** `docs/public/executors.md`, `docs/public/k8s.md`,
  `docs/public/feature-status.md`, optional `docs/public/security.md`,
  `k8s/executor-rbac.yaml`, and scoped `AGENTS.md` files.
- **Dependencies:** Tasks 02, 03, and 05 so docs describe final behavior.
- **Parallelism:** parallel-safe with Task 06 after dependencies; owns docs,
  manifests, and guidance files only.
- **Inputs:** full spec, ADR, final config/API/UI names, docs-maintainer rules.

## Results

Completed the Kubernetes executor public documentation and opt-in namespaced
RBAC example.

- Updated `docs/public/executors.md`, `docs/public/k8s.md`,
  `docs/public/feature-status.md`, and `docs/public/security.md`. The Kubernetes
  page remains a how-to guide; the executor and feature-status pages are
  reference material, and the security addition is explanation.
- Added an explicit administrator trust warning that kubeconfig exec credential
  helpers run with backend privileges, with root-owned/read-only file guidance
  and review requirements for exec and auth-provider plugins.
- The integration audit also updated exhaustive executor references in
  `docs/public/architecture.md`, `docs/public/operations.md`,
  `docs/public/developer-tools.md`, and `docs/public/tasks-and-workflows.md` so
  runtime, editor, workspace-source, metrics, credential, and cleanup behavior
  no longer omits Kubernetes.
- Updated `docs/public/coverage.json` with the shipped Kubernetes executor
  backend, UI, manifest, unit, and E2E evidence. No navigation change was
  required because every changed page already had a published slug.
- Added `k8s/executor-rbac.yaml`. It is not referenced by the control-plane
  apply command. Its Role grants Pod `get/create/delete/watch`, Pod
  `exec`/`portforward` `get/create`, and the conditional managed-PVC
  `get/create/delete` rule, with no Namespace or Pod-list permission.
- Updated root and backend `AGENTS.md` guidance so the containers E2E project,
  Kubernetes packages, runtime backend, and exact lifecycle contract are
  discoverable to future contributors.

Validation:

- `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check`
  passed: 61 tests, 41 published pages validated, and no whitespace errors.
- `kubectl apply --dry-run=client --validate=false -f k8s/executor-rbac.yaml`
  passed for the ServiceAccount, Role, and RoleBinding.
- `kubectl create --dry-run=client -o json -f k8s/executor-rbac.yaml | jq -se 'length == 3 and all(.[]; .metadata.namespace == "kandev-agents") and (map(select(.kind == "Role"))[0].rules | map([.resources, .verbs])) == [[["pods"],["get","create","delete","watch"]],[["pods/exec","pods/portforward"],["get","create"]],[["persistentvolumeclaims"],["get","create","delete"]]]'`
  returned `true`, confirming three namespaced objects and the exact ordered
  Pod, streaming-subresource, and PVC rules.
- The optional Landing publication build was not run because no sibling
  `landing` checkout is available in this workspace.
