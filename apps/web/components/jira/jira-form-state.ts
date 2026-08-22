import type { TFunction } from "i18next";
import type { JiraAuthMethod, JiraConfig, JiraInstanceType } from "@/lib/types/jira";

export type FormState = {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  instanceType: JiraInstanceType;
  defaultProjectKey: string;
  secret: string;
  clientId: string;
};

export const emptyForm: FormState = {
  siteUrl: "",
  email: "",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "",
  secret: "",
  clientId: "",
};

export function configToForm(cfg: JiraConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    siteUrl: cfg.siteUrl,
    email: cfg.email,
    authMethod: cfg.authMethod,
    // Legacy rows written before Server/DC support carry an empty instanceType;
    // default to cloud so the dropdown has a valid selection.
    instanceType: cfg.instanceType || "cloud",
    defaultProjectKey: cfg.defaultProjectKey,
    secret: "",
    clientId: cfg.clientId || "",
  };
}

function normalizeComparableSiteUrl(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) return "";
  return trimmed.includes("://") ? trimmed : `https://${trimmed}`;
}

// savedSecretMatches reports whether the saved secret can be reused against
// the current form values. Reuse is only safe when every identity component
// of the saved credential still matches: same auth method, same instance
// type, same Jira host, and — for Cloud api_token where the basic pair is
// email:token — the same email (case-insensitive). Otherwise the user could
// change the site URL or Cloud account and silently submit the previous
// token to a different host/account.
export function savedSecretMatches(config: JiraConfig | null, form: FormState): boolean {
  if (!config?.hasSecret) return false;
  if (config.authMethod !== form.authMethod) return false;
  if ((config.instanceType || "cloud") !== form.instanceType) return false;
  if (normalizeComparableSiteUrl(config.siteUrl) !== normalizeComparableSiteUrl(form.siteUrl)) {
    return false;
  }
  if (form.authMethod !== "api_token") return true;
  return (config.email ?? "").toLowerCase() === form.email.toLowerCase();
}

export type DerivedFormState = {
  isOAuth: boolean;
  savedSecretMatchesMode: boolean;
  disableSave: boolean;
  disableTest: boolean;
  revision: string;
  dirty: boolean;
  invalidReason: string | undefined;
};

// The subset of the settings hook's state deriveFormState reads. Structural, so
// this module stays independent of the component's hook return type.
type FormStateInputs = {
  config: JiraConfig | null;
  form: FormState;
  saving: boolean;
  loading: boolean;
};

function jiraDisableSave(
  s: FormStateInputs,
  emailRequired: boolean,
  missingSecret: boolean,
  oauthNeedsConnect: boolean,
): boolean {
  return (
    s.saving ||
    !s.form.siteUrl ||
    (emailRequired && !s.form.email) ||
    missingSecret ||
    oauthNeedsConnect
  );
}

function jiraInvalidReason(
  form: FormState,
  emailRequired: boolean,
  missingSecret: boolean,
  t: TFunction,
): string | undefined {
  if (!form.siteUrl) return t("jira:aJiraSiteUrlIsRequired");
  if (emailRequired && !form.email) return t("jira:anEmailAddressIsRequired");
  if (missingSecret) return t("jira:aCredentialIsRequired");
  return undefined;
}

// Derives save/test gating and the validation message from the form + stored
// config. Extracted so JiraConnectionSection stays under the complexity limit.
export function deriveFormState(s: FormStateInputs, t: TFunction): DerivedFormState {
  const savedSecretMatchesMode = savedSecretMatches(s.config, s.form);
  const isOAuth = s.form.authMethod === "oauth";
  const missingSecret = !isOAuth && !savedSecretMatchesMode && !s.form.secret;
  const oauthNeedsConnect = isOAuth && !savedSecretMatchesMode;
  const emailRequired = s.form.instanceType === "cloud" && s.form.authMethod === "api_token";
  const disableSave = jiraDisableSave(s, emailRequired, missingSecret, oauthNeedsConnect);
  const disableTest = missingSecret || (isOAuth && oauthNeedsConnect);
  const revision = JSON.stringify(s.form);
  const dirty = !s.loading && revision !== JSON.stringify(configToForm(s.config));
  const invalidReason = jiraInvalidReason(
    s.form,
    emailRequired,
    missingSecret || oauthNeedsConnect,
    t,
  );
  return {
    isOAuth,
    savedSecretMatchesMode,
    disableSave,
    disableTest,
    revision,
    dirty,
    invalidReason,
  };
}
