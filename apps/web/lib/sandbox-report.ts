import type { SandboxReport } from "./api/domains/sandbox-reports-api";
import type { ExecutionSandboxSnapshot, ExecutionSnapshot } from "./types/execution";

export function isSandboxReportBound(
  report: SandboxReport,
  snapshot: ExecutionSnapshot | null | undefined,
) {
  const sandbox = snapshot?.sandbox;
  return Boolean(
    sandbox &&
    report.schema_version === 1 &&
    Number.isSafeInteger(report.version) &&
    report.version > 0 &&
    report.attempt_id === snapshot?.attempt_id &&
    report.session_id === snapshot?.session_id &&
    report.revision_id === sandbox.revision_id &&
    report.policy_digest === sandbox.policy_digest &&
    report.ceiling_digest === sandbox.ceiling_digest &&
    isProviderBound(report.provider, sandbox),
  );
}

function isProviderBound(provider: SandboxReport["provider"], sandbox: ExecutionSandboxSnapshot) {
  return (
    provider.installation_id === sandbox.installation_id &&
    provider.key === sandbox.provider_key &&
    provider.version === sandbox.provider_version &&
    provider.package_digest === sandbox.package_digest &&
    provider.runtime_version === sandbox.runtime_version &&
    provider.runtime_digest === sandbox.runtime_digest
  );
}
