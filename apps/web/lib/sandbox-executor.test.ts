import { describe, expect, it } from "vitest";
import { executorPolicyReady } from "./sandbox-executor";
import type { SandboxExecutorPolicy, SandboxRevision } from "./types/sandbox";

const revision = {
  id: "ceiling",
  revoked: false,
  policy: { installation_id: "install", provider_key: "alpha" },
} as SandboxRevision;
const policy: SandboxExecutorPolicy = {
  mode: "required",
  installation_id: "install",
  provider_key: "alpha",
  ceiling_revision_id: "ceiling",
  default_revision_id: "ceiling",
  eligible_revision_ids: ["ceiling"],
};

describe("executor policy form readiness", () => {
  it("permits workspace eligibility but keeps global defaults and ceilings", () => {
    const workspace = {
      ...revision,
      id: "workspace",
      policy: { ...revision.policy, workspace_id: "workspace-1" },
    };
    const eligible = { ...policy, eligible_revision_ids: ["ceiling", "workspace"] };
    expect(executorPolicyReady(eligible, [revision, workspace])).toBe(true);
    expect(
      executorPolicyReady({ ...eligible, default_revision_id: "workspace" }, [revision, workspace]),
    ).toBe(false);
    expect(
      executorPolicyReady({ ...eligible, ceiling_revision_id: "workspace" }, [revision, workspace]),
    ).toBe(false);
  });
  it("requires an explicit provider, ceiling and eligible default", () => {
    expect(executorPolicyReady(policy, [revision])).toBe(true);
    expect(executorPolicyReady({ ...policy, default_revision_id: "" }, [revision])).toBe(false);
    expect(executorPolicyReady({ ...policy, eligible_revision_ids: [] }, [revision])).toBe(false);
    expect(executorPolicyReady({ ...policy, ceiling_revision_id: "missing" }, [revision])).toBe(
      false,
    );
  });
  it("preserves invalid input for repair without accepting revoked or foreign revisions", () => {
    expect(executorPolicyReady(policy, [{ ...revision, revoked: true }])).toBe(false);
    expect(executorPolicyReady({ ...policy, provider_key: "beta" }, [revision])).toBe(false);
    expect(
      executorPolicyReady({ ...policy, eligible_revision_ids: ["ceiling", "missing"] }, [revision]),
    ).toBe(false);
    expect(policy.eligible_revision_ids).toEqual(["ceiling"]);
  });
  it("accepts disabled mode only when enforcement references are explicitly cleared", () => {
    expect(executorPolicyReady({ ...policy, mode: "disabled" }, [revision])).toBe(false);
    expect(
      executorPolicyReady(
        {
          mode: "disabled",
          installation_id: "",
          provider_key: "",
          ceiling_revision_id: "",
          default_revision_id: "",
          eligible_revision_ids: [],
        },
        [],
      ),
    ).toBe(true);
  });
});
