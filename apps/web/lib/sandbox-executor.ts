import type { SandboxExecutorPolicy, SandboxRevision } from "./types/sandbox";

export function executorPolicyReady(policy: SandboxExecutorPolicy, revisions: SandboxRevision[]) {
  if (policy.mode === "disabled")
    return (
      !policy.installation_id &&
      !policy.provider_key &&
      !policy.ceiling_revision_id &&
      !policy.default_revision_id &&
      policy.eligible_revision_ids.length === 0
    );
  if (
    !policy.installation_id ||
    !policy.provider_key ||
    !policy.eligible_revision_ids.includes(policy.default_revision_id)
  )
    return false;
  return [policy.ceiling_revision_id, ...policy.eligible_revision_ids].every((id) =>
    revisions.some(
      (item) =>
        item.id === id &&
        !item.revoked &&
        item.policy.installation_id === policy.installation_id &&
        item.policy.provider_key === policy.provider_key,
    ),
  );
}

export function emptyExecutorPolicy(mode: SandboxExecutorPolicy["mode"]): SandboxExecutorPolicy {
  return {
    mode,
    installation_id: "",
    provider_key: "",
    ceiling_revision_id: "",
    default_revision_id: "",
    eligible_revision_ids: [],
  };
}
