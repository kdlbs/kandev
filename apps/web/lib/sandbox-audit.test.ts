import { expect, it } from "vitest";
import { isAuditSignerTrustedNow } from "./sandbox-audit";

const key = "a".repeat(64);
const trust = { id: "selected-revision", version: 2, fingerprint: key, revoked: false };
const view = {
  signer_trusted_now: true,
  verification: {
    state: "completed",
    trust: { revision_id: trust.id, version: trust.version, fingerprint: key },
    result: { signature_state: "verified", signer_key_fingerprint: key },
  },
} as const;

it("keeps signature history separate from current operator trust", () => {
  expect(isAuditSignerTrustedNow(view, trust)).toBe(true);
  expect(isAuditSignerTrustedNow(view, { ...trust, revoked: true })).toBe(false);
  expect(isAuditSignerTrustedNow(view, { ...trust, version: 3 })).toBe(false);
  expect(isAuditSignerTrustedNow(view, { ...trust, id: "new-revision" })).toBe(false);
  expect(isAuditSignerTrustedNow(view, { ...trust, fingerprint: "b".repeat(64) })).toBe(false);
  expect(isAuditSignerTrustedNow(view, null)).toBe(false);
  expect(isAuditSignerTrustedNow({ ...view, signer_trusted_now: false }, trust)).toBe(false);
  expect(
    isAuditSignerTrustedNow(
      {
        ...view,
        verification: {
          ...view.verification,
          result: { ...view.verification.result, signature_state: "invalid" },
        },
      },
      trust,
    ),
  ).toBe(false);
  expect(view.verification.result.signature_state).toBe("verified");
});
