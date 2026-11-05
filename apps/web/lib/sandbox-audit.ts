import type {
  SandboxAuditJob,
  SandboxAuditTrust,
  SandboxAuditView,
} from "./api/domains/sandbox-audit-api";
import type { SandboxReport } from "./api/domains/sandbox-reports-api";

type AuditReportBinding = Pick<
  SandboxReport,
  "attempt_id" | "session_id" | "digest" | "provider"
> & { data: Pick<SandboxReport["data"], "finalized" | "final_audit"> };

export function auditReportBinding(report: SandboxReport): AuditReportBinding {
  return {
    attempt_id: report.attempt_id,
    session_id: report.session_id,
    digest: report.digest,
    provider: report.provider,
    data: { finalized: report.data.finalized, final_audit: report.data.final_audit },
  };
}

export function isAuditTrustBound(trust: SandboxAuditTrust, report: AuditReportBinding): boolean {
  return Boolean(
    trust?.schema_version === 1 &&
    Number.isSafeInteger(trust.version) &&
    trust.version >= 0 &&
    trust.installation_id === report.provider.installation_id &&
    trust.provider_key === report.provider.key &&
    typeof trust.revoked === "boolean" &&
    typeof trust.public_key_pem === "string" &&
    trust.public_key_pem.length <= 16_384 &&
    validAuditTrustKey(trust),
  );
}

function validAuditTrustKey(trust: SandboxAuditTrust): boolean {
  if (trust.revoked) return trust.public_key_pem === "" && trust.fingerprint === "";
  return (
    trust.public_key_pem.startsWith("-----BEGIN PUBLIC KEY-----") &&
    !trust.public_key_pem.includes("PRIVATE KEY") &&
    /^[a-f0-9]{64}$/.test(trust.fingerprint)
  );
}

export function isAuditViewBound(view: SandboxAuditView, report: AuditReportBinding): boolean {
  const job = view?.verification;
  const receipt = report.data.final_audit;
  return Boolean(
    receipt &&
    report.data.finalized &&
    job?.schema_version === 1 &&
    job.id &&
    validAuditVersion(job.version) &&
    typeof view.signer_trusted_now === "boolean" &&
    job.attempt_id === report.attempt_id &&
    job.session_id === report.session_id &&
    job.report_digest === report.digest &&
    job.artifact_set_digest === receipt.artifact_set_digest &&
    job.binding_digest === receipt.binding_digest &&
    ["pending", "running", "completed", "failed", "interrupted"].includes(job.state) &&
    sameAuditProvider(job.provider, report.provider),
  );
}

function validAuditVersion(version: number): boolean {
  return Number.isSafeInteger(version) && version > 0;
}

function sameAuditProvider(
  provider: SandboxReport["provider"],
  expected: SandboxReport["provider"],
): boolean {
  return Boolean(
    provider &&
    provider.plugin_id === expected.plugin_id &&
    provider.installation_id === expected.installation_id &&
    provider.key === expected.key &&
    provider.version === expected.version &&
    provider.package_digest === expected.package_digest &&
    provider.runtime_version === expected.runtime_version &&
    provider.runtime_digest === expected.runtime_digest &&
    (provider.protocol_major ?? 1) === (expected.protocol_major ?? 1),
  );
}

type AuditTrustView = {
  signer_trusted_now: boolean;
  verification: Pick<SandboxAuditJob, "state"> & {
    trust: Pick<SandboxAuditJob["trust"], "revision_id" | "version" | "fingerprint">;
    result?: Pick<
      NonNullable<SandboxAuditJob["result"]>,
      "signature_state" | "signer_key_fingerprint"
    >;
  };
};

export function isAuditSignerTrustedNow(
  view: AuditTrustView,
  trust: Pick<SandboxAuditTrust, "id" | "version" | "fingerprint" | "revoked"> | null,
): boolean {
  const job = view.verification;
  return Boolean(
    view.signer_trusted_now &&
    trust &&
    !trust.revoked &&
    job.state === "completed" &&
    job.result?.signature_state === "verified" &&
    trust.id === job.trust.revision_id &&
    trust.version === job.trust.version &&
    trust.fingerprint === job.trust.fingerprint &&
    trust.fingerprint === job.result.signer_key_fingerprint,
  );
}
