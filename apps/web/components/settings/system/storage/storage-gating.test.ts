import { describe, expect, it } from "vitest";
import {
  policyBlockedReason,
  policyControlsPending,
  storageActionDisabledReason,
} from "./storage-gating";

const t = (key: string) => key;
const ADMIN_ONLY = "system:storageAdminOnly";
const ACTION_PENDING = "system:storageActionPending";

describe("storageActionDisabledReason", () => {
  it("blocks every maintenance action for a non-admin", () => {
    expect(storageActionDisabledReason(t, null, false)).toBe(ADMIN_ONLY);
  });

  it("reports the admin gate ahead of a pending action", () => {
    expect(storageActionDisabledReason(t, "save", false)).toBe(ADMIN_ONLY);
  });

  it("keeps the pending reason for an admin", () => {
    expect(storageActionDisabledReason(t, "save", true)).toBe(ACTION_PENDING);
  });

  it("leaves an idle admin unblocked", () => {
    expect(storageActionDisabledReason(t, null, true)).toBeUndefined();
  });
});

describe("policyBlockedReason", () => {
  it("blocks saving the policy for a non-admin", () => {
    expect(policyBlockedReason(t, null, false, false)).toBe(ADMIN_ONLY);
  });

  it("blocks a non-admin even while the policy is still loading", () => {
    expect(policyBlockedReason(t, null, true, false)).toBe(ADMIN_ONLY);
  });

  it("keeps the adoption-pending reason for an admin", () => {
    expect(policyBlockedReason(t, "adopt", false, true)).toBe("system:storageAdoptionPending");
  });

  it("keeps the loading reason for an admin", () => {
    expect(policyBlockedReason(t, null, true, true)).toBe("system:storagePolicyLoadingBlock");
  });

  it("leaves a ready admin unblocked", () => {
    expect(policyBlockedReason(t, null, false, true)).toBeUndefined();
  });
});

describe("policyControlsPending", () => {
  it("locks the policy controls while a save or adoption is in flight", () => {
    expect(policyControlsPending("save", undefined)).toBe(true);
    expect(policyControlsPending("adopt", undefined)).toBe(true);
  });

  it("leaves the controls live for an idle admin", () => {
    expect(policyControlsPending(null, undefined)).toBe(false);
    expect(policyControlsPending("analyze", undefined)).toBe(false);
  });

  it("locks the controls whenever a read-only reason applies", () => {
    expect(policyControlsPending(null, ADMIN_ONLY)).toBe(true);
  });
});
