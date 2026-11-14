import { describe, expect, it } from "vitest";
import type { MergeRepo, ProtectionObservation } from "@/lib/api/domains/coordinator-merge-api";
import {
  buildMergeChanges,
  effectivePolicy,
  hasUnknownProtection,
  isMergeDirty,
  isPrincipalInactive,
  markReplacePrincipal,
  protectionOf,
  pruneDraft,
  sameStoredPolicies,
  setPolicy,
} from "./merge-draft";

function repo(id: string, policy: MergeRepo["policy"], active = true): MergeRepo {
  return { repository_id: id, name: id, policy, automatic_active: active };
}

const denied = repo("r1", "denied");
const inactive = repo("r2", "automatic", false);

describe("setPolicy", () => {
  it("records a change and drops it when the stored policy is chosen again", () => {
    const draft = setPolicy({}, denied, "automatic");
    expect(effectivePolicy(denied, draft)).toBe("automatic");
    expect(setPolicy(draft, denied, "denied")).toEqual({});
  });
});

describe("replace principal", () => {
  it("is only a change for an inactive automatic repository", () => {
    expect(isPrincipalInactive(inactive)).toBe(true);
    expect(isPrincipalInactive(repo("r3", "automatic", true))).toBe(false);
    const draft = markReplacePrincipal({}, inactive);
    expect(isMergeDirty(draft, [inactive])).toBe(true);
    expect(buildMergeChanges(draft, [inactive])).toEqual([
      { repository_id: "r2", policy: "automatic", replace_principal: true },
    ]);
  });

  it("is not sent once the principal is active again", () => {
    const draft = markReplacePrincipal({}, inactive);
    const healed = repo("r2", "automatic", true);
    expect(isMergeDirty(draft, [healed])).toBe(false);
    expect(buildMergeChanges(draft, [healed])).toEqual([]);
    expect(pruneDraft(draft, [healed])).toEqual({});
  });
});

describe("buildMergeChanges", () => {
  it("lists only differing repositories in repository order", () => {
    const repos = [denied, repo("r4", "requires_approval"), inactive];
    let draft = setPolicy({}, repos[1], "automatic");
    draft = setPolicy(draft, denied, "requires_approval");
    expect(buildMergeChanges(draft, repos)).toEqual([
      { repository_id: "r1", policy: "requires_approval" },
      { repository_id: "r4", policy: "automatic" },
    ]);
  });

  it("prunes edits for repositories that vanished", () => {
    const draft = setPolicy({}, denied, "automatic");
    expect(pruneDraft(draft, [])).toEqual({});
  });
});

describe("protection", () => {
  const obs = (branch: string, pipeline: ProtectionObservation["pipeline"]) =>
    ({
      repository_id: "r1",
      branch,
      pipeline,
      approvals: "enforced",
      threads: "enforced",
      observed_at: "2026-01-01T00:00:00Z",
    }) as ProtectionObservation;

  it("orders one repository's branches ascending and ignores others", () => {
    const all = [
      obs("main", "enforced"),
      obs("dev", "enforced"),
      { ...obs("a", "enforced"), repository_id: "r9" },
    ];
    expect(protectionOf(all, "r1").map((o) => o.branch)).toEqual(["dev", "main"]);
  });

  it("detects any unknown reading", () => {
    expect(hasUnknownProtection([obs("main", "enforced")])).toBe(false);
    expect(hasUnknownProtection([obs("main", "unknown")])).toBe(true);
  });
});

describe("sameStoredPolicies", () => {
  it("compares only the named repositories", () => {
    const before = [denied, repo("r5", "denied")];
    const after = [repo("r1", "denied"), repo("r5", "automatic")];
    expect(sameStoredPolicies(["r1"], before, after)).toBe(true);
    expect(sameStoredPolicies(["r5"], before, after)).toBe(false);
    expect(sameStoredPolicies(["gone"], before, after)).toBe(false);
  });
});
