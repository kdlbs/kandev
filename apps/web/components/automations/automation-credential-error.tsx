import { useTranslation } from "react-i18next";
import { WebSocketRequestError } from "@/lib/ws/request-error";

export type AutomationCredentialErrorTarget =
  | "agentProfileId"
  | "executorProfileId"
  | "repositorySelections";

export type AutomationCredentialError = {
  field: string;
  source: string;
  target: AutomationCredentialErrorTarget;
};

const CREDENTIAL_ERROR_CODE = "automation_credentials_unavailable";
const CREDENTIAL_STORAGE_ERROR_CODE = "automation_credential_storage_unavailable";
type Translate = (key: string, options?: Record<string, unknown>) => string;

function credentialErrorTarget(source: string): AutomationCredentialErrorTarget {
  const normalizedSource = source.toLowerCase();
  if (normalizedSource.startsWith("executor")) return "executorProfileId";
  if (
    normalizedSource.startsWith("repository") ||
    normalizedSource.startsWith("selected repository")
  ) {
    return "repositorySelections";
  }
  return "agentProfileId";
}

export function parseAutomationCredentialError(error: unknown): AutomationCredentialError | null {
  if (!(error instanceof WebSocketRequestError)) return null;
  if (error.code !== "VALIDATION_ERROR") return null;
  const details = error.details;
  if (details?.credential_error_code !== CREDENTIAL_ERROR_CODE) return null;
  const field = typeof details.field === "string" ? details.field : "";
  const source = typeof details.source === "string" ? details.source : "";
  if (!field || !source) return null;

  return { field, source, target: credentialErrorTarget(source) };
}

export function isAutomationCredentialStorageError(error: unknown): boolean {
  return (
    error instanceof WebSocketRequestError &&
    error.code === "UNAVAILABLE" &&
    error.details?.credential_error_code === CREDENTIAL_STORAGE_ERROR_CODE
  );
}

export function parseAutomationCredentialRunError(
  errorMessage: string | undefined,
): AutomationCredentialError | null {
  if (!errorMessage?.startsWith(`${CREDENTIAL_ERROR_CODE}: `)) return null;
  const body = errorMessage.slice(CREDENTIAL_ERROR_CODE.length + 2);
  const suffix = " is unavailable";
  if (!body.endsWith(suffix)) return null;
  const sourceSeparator = body.lastIndexOf(" from ");
  if (sourceSeparator <= 0) return null;
  const field = body.slice(0, sourceSeparator);
  const source = body.slice(sourceSeparator + 6, -suffix.length);
  if (!field || !source) return null;
  return { field, source, target: credentialErrorTarget(source) };
}

function localizedSource(error: AutomationCredentialError, t: Translate) {
  if (error.target === "agentProfileId") return t("automations:agentProfileLabel");
  if (error.target === "executorProfileId") return t("automations:executorProfileLabel");
  const normalizedSource = error.source.toLowerCase();
  if (normalizedSource.startsWith("repository ")) {
    return t("automations:credentialRepositorySource", {
      name: error.source.slice("repository ".length),
    });
  }
  return t("automations:credentialSelectedRepositorySource");
}

function credentialRecovery(
  error: AutomationCredentialError,
  context: "editor" | "run",
  t: Translate,
) {
  if (context === "run") return t("automations:credentialRunRecovery");
  if (error.target === "repositorySelections") return t("automations:credentialChooseRepository");
  return t("automations:credentialChooseProfile");
}

export function automationCredentialErrorSummary(
  error: AutomationCredentialError,
  t: Translate,
): string {
  return t("automations:credentialUnavailable", {
    field: error.field,
    source: localizedSource(error, t),
  });
}

export function AutomationCredentialErrorMessage({
  error,
  context = "editor",
  id,
}: {
  error: AutomationCredentialError;
  context?: "editor" | "run";
  id?: string;
}) {
  const { t } = useTranslation();
  const translate: Translate = (key, options) => String(t(key, options));
  return (
    <div
      className="space-y-1 text-sm text-destructive"
      role="alert"
      id={id}
      data-testid="automation-credential-error"
      data-credential-error-target={error.target}
    >
      <p>{automationCredentialErrorSummary(error, translate)}</p>
      <p>{credentialRecovery(error, context, translate)}</p>
    </div>
  );
}
