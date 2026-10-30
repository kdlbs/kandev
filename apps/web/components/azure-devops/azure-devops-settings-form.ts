import type { IntegrationAuthHealth } from "@/components/integrations/auth-status-banner";
import type {
  AzureDevOpsAuthMethod,
  AzureDevOpsConfig,
  SetAzureDevOpsConfigRequest,
  StartAzureDevOpsOAuthRequest,
} from "@/lib/types/azure-devops";

export type FormState = {
  organizationUrl: string;
  defaultProjectId: string;
  defaultProjectName: string;
  authMethod: AzureDevOpsAuthMethod;
  pat: string;
  oauthClientId: string;
};

export const EMPTY_FORM: FormState = {
  organizationUrl: "",
  defaultProjectId: "",
  defaultProjectName: "",
  authMethod: "pat",
  pat: "",
  oauthClientId: "",
};

export function configToForm(config: AzureDevOpsConfig | null): FormState {
  if (!config) return EMPTY_FORM;
  return {
    organizationUrl: config.organizationUrl,
    defaultProjectId: config.defaultProjectId ?? "",
    defaultProjectName: config.defaultProjectName ?? "",
    authMethod: config.authMethod,
    pat: "",
    oauthClientId: config.oauthClientId ?? "",
  };
}

/** Applies one field edit; a new organization clears its default project. */
export function withFieldUpdate<K extends keyof FormState>(
  current: FormState,
  key: K,
  value: FormState[K],
): FormState {
  const organizationChanged =
    key === "organizationUrl" &&
    normalizedOrganization(String(value)) !== normalizedOrganization(current.organizationUrl);
  return {
    ...current,
    [key]: value,
    ...(organizationChanged ? { defaultProjectId: "", defaultProjectName: "" } : {}),
  };
}

export function configToHealth(config: AzureDevOpsConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasSecret) return null;
  return {
    ok: config.lastOk,
    error: config.lastError ?? "",
    checkedAt: config.lastCheckedAt ? new Date(config.lastCheckedAt) : null,
  };
}

export function normalizedOrganization(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

/** Returns the canonical organization name of a dev.azure.com URL, or null. */
export function azureDevOpsOrganization(value: string): string | null {
  let organizationURL: URL;
  try {
    organizationURL = new URL(value.trim());
  } catch {
    return null;
  }
  const organization = organizationURL.pathname.split("/").filter(Boolean);
  if (
    organizationURL.protocol !== "https:" ||
    organizationURL.hostname.toLowerCase() !== "dev.azure.com" ||
    organizationURL.port ||
    organizationURL.username ||
    organizationURL.password ||
    organizationURL.search ||
    organizationURL.hash ||
    organization.length !== 1 ||
    !/^[a-z0-9-]+$/i.test(organization[0])
  ) {
    return null;
  }
  return organization[0];
}

export function azureDevOpsPATURL(value: string): string | null {
  const organization = azureDevOpsOrganization(value);
  return organization ? `https://dev.azure.com/${organization}/_usersSettings/tokens` : null;
}

function sameOrganization(config: AzureDevOpsConfig, form: FormState): boolean {
  return (
    normalizedOrganization(config.organizationUrl) === normalizedOrganization(form.organizationUrl)
  );
}

export function savedPATMatches(config: AzureDevOpsConfig | null, form: FormState): boolean {
  return !!config?.hasSecret && config.authMethod === "pat" && sameOrganization(config, form);
}

/** A stored Microsoft account credential exists for the form's organization. */
export function signedInWithMicrosoft(config: AzureDevOpsConfig | null, form: FormState): boolean {
  return !!config?.hasSecret && config.authMethod === "oauth" && sameOrganization(config, form);
}

/** Whether Test connection and Save can run with the form's method. */
export function canSubmitForm(config: AzureDevOpsConfig | null, form: FormState): boolean {
  if (!form.organizationUrl) return false;
  if (form.authMethod === "oauth") return signedInWithMicrosoft(config, form);
  return savedPATMatches(config, form) || !!form.pat;
}

export function requestFromForm(
  form: FormState,
  savedConfig: AzureDevOpsConfig | null,
): SetAzureDevOpsConfigRequest {
  const organizationMatches =
    normalizedOrganization(form.organizationUrl) ===
    normalizedOrganization(savedConfig?.organizationUrl ?? "");
  const request: SetAzureDevOpsConfigRequest = {
    organizationUrl: normalizedOrganization(form.organizationUrl),
    defaultProjectId: organizationMatches ? form.defaultProjectId : undefined,
    defaultProjectName: organizationMatches ? form.defaultProjectName : undefined,
    authMethod: form.authMethod,
  };
  if (form.authMethod === "pat") request.pat = form.pat || undefined;
  return request;
}

/**
 * Microsoft sign-in request. Sign-in always returns to the page's own origin,
 * so a changed port or host needs no configuration. A blank client ID lets the
 * backend apply the installation's default application.
 */
export function startRequestFromForm(
  form: FormState,
  origin: string,
): StartAzureDevOpsOAuthRequest {
  const request: StartAzureDevOpsOAuthRequest = {
    organizationUrl: normalizedOrganization(form.organizationUrl),
    defaultProjectId: form.defaultProjectId || undefined,
    defaultProjectName: form.defaultProjectName || undefined,
    redirectUrl: origin,
  };
  const clientId = form.oauthClientId.trim();
  if (clientId) request.clientId = clientId;
  return request;
}
