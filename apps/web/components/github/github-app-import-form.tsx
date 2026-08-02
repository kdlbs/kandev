"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import type { useGitHubAppRegistrations } from "@/hooks/domains/github/use-github-app-registrations";
import type { GitHubAppOwnerType, PrepareGitHubAppImportResponse } from "@/lib/types/github";
import {
  appRegistrationError,
  defaultVisibility,
  githubAppSettingsURL,
  normalizePublicBaseUrl,
  validateImportSecrets,
  validateRegistrationBasics,
  type AppSetupErrors,
} from "./github-app-onboarding-model";
import { GitHubAppImportGuide } from "./github-app-import-guide";
import {
  GitHubAppImportIdentityFields,
  GitHubAppImportSecretFields,
  type GitHubAppImportValues,
} from "./github-app-import-fields";
import { GitHubAppVisibilityField } from "./github-app-visibility-field";
import { useTranslation } from "react-i18next";

type RegistrationHook = ReturnType<typeof useGitHubAppRegistrations>;

export function GitHubAppImportForm({
  workspaceId,
  registrations,
  onImported,
}: {
  workspaceId: string;
  registrations: RegistrationHook;
  onImported: (registrationId: string) => void;
}) {
  const { t } = useTranslation();
  const [preparation, setPreparation] = useState<PrepareGitHubAppImportResponse | null>(null);
  const [values, setValues] = useState(initialValues);
  const [errors, setErrors] = useState<AppSetupErrors>({});
  const { toast } = useToast();
  useEffect(() => {
    setPreparation(null);
    setValues(initialValues);
    setErrors({});
  }, [workspaceId]);
  const update = (name: keyof typeof values, value: string) =>
    setValues((current) => ({ ...current, [name]: value }));
  async function prepare(event: React.FormEvent) {
    event.preventDefault();
    const nextErrors: AppSetupErrors = {};
    const publicBaseUrl = normalizePublicBaseUrl(values.publicBaseUrl, nextErrors);
    setErrors(nextErrors);
    if (!publicBaseUrl) return;
    try {
      setPreparation(await registrations.prepareImport({ public_base_url: publicBaseUrl }));
    } catch (error) {
      showError(error, t("github:importSetupCouldNotStart"), toast);
    }
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!preparation) return;
    const ownerType = values.ownerType as GitHubAppOwnerType;
    const basics = validateRegistrationBasics({ ...values, ownerType });
    const secrets = validateImportSecrets(values);
    const nextErrors = { ...basics.errors, ...secrets.errors };
    setErrors(nextErrors);
    if (!basics.publicBaseUrl || Object.keys(nextErrors).length) return;
    try {
      const imported = await registrations.importRegistration({
        registration_id: preparation.registration_id,
        display_name: basics.displayName,
        github_host: "github.com",
        app_id: secrets.appId,
        client_id: values.clientId.trim(),
        client_secret: values.clientSecret,
        private_key: values.privateKey.trim(),
        webhook_secret: values.webhookSecret,
        slug: values.slug.trim(),
        owner_login: basics.ownerLogin,
        owner_type: ownerType,
        visibility: values.visibility,
        public_base_url: basics.publicBaseUrl,
      });
      setValues(clearImportSecrets);
      toast({
        description: t("github:githubAppImportedItIsReady"),
        variant: "success",
      });
      onImported(imported.id);
    } catch (error) {
      showError(error, t("github:githubAppCouldNotBeImported"), toast);
    }
  }

  if (!preparation) {
    return (
      <PrepareImportForm
        publicBaseUrl={values.publicBaseUrl}
        error={errors.publicBaseUrl}
        mutating={registrations.mutating}
        onPublicBaseUrl={(value) => update("publicBaseUrl", value)}
        onSubmit={prepare}
      />
    );
  }

  const settingsUrl = importSettingsUrl(values);
  const startOver = () => {
    setPreparation(null);
    setErrors({});
    setValues(clearImportSecrets);
  };
  return (
    <PreparedImportForm
      preparation={preparation}
      settingsUrl={settingsUrl}
      values={values}
      errors={errors}
      mutating={registrations.mutating}
      update={update}
      onVisibility={(visibility) => setValues((current) => ({ ...current, visibility }))}
      onStartOver={startOver}
      onSubmit={submit}
    />
  );
}

function PrepareImportForm(props: {
  publicBaseUrl: string;
  error?: string;
  mutating: boolean;
  onPublicBaseUrl: (value: string) => void;
  onSubmit: (event: React.FormEvent) => void;
}) {
  const { t } = useTranslation();
  return (
    <form onSubmit={props.onSubmit} className="space-y-3">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:addAnExistingGithubApp")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:kandevFirstReservesExactCallbackAnd")}
        </p>
      </div>
      <Field label={t("github:publicKandevUrl")} error={props.error}>
        <Input
          className="h-11"
          type="url"
          placeholder="https://kandev.example.com"
          value={props.publicBaseUrl}
          onChange={(event) => props.onPublicBaseUrl(event.target.value)}
        />
      </Field>
      <Button type="submit" disabled={props.mutating} className="h-11 cursor-pointer">
        {props.mutating && <Spinner className="mr-2 h-4 w-4" />}
        {t("github:generateSetupInstructions")}
      </Button>
    </form>
  );
}

type PreparedImportProps = {
  preparation: PrepareGitHubAppImportResponse;
  settingsUrl?: string;
  values: GitHubAppImportValues;
  errors: AppSetupErrors;
  mutating: boolean;
  update: (name: keyof GitHubAppImportValues, value: string) => void;
  onVisibility: (value: "private" | "public") => void;
  onStartOver: () => void;
  onSubmit: (event: React.FormEvent) => void;
};

function PreparedImportForm(props: PreparedImportProps) {
  const { t } = useTranslation();
  return (
    <form onSubmit={props.onSubmit} className="space-y-5">
      <GitHubAppImportIdentityFields
        values={props.values}
        errors={props.errors}
        update={props.update}
      />
      <GitHubAppImportGuide preparation={props.preparation} settingsUrl={props.settingsUrl} />
      <GitHubAppVisibilityField value={props.values.visibility} onChange={props.onVisibility} />
      <GitHubAppImportSecretFields
        values={props.values}
        errors={props.errors}
        update={props.update}
      />
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button type="submit" disabled={props.mutating} className="h-11 cursor-pointer">
          {props.mutating && <Spinner className="mr-2 h-4 w-4" />}
          {t("github:verifyAndImportApp")}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="h-11 cursor-pointer"
          onClick={props.onStartOver}
        >
          {t("github:startOver")}
        </Button>
      </div>
    </form>
  );
}

const initialValues: GitHubAppImportValues = {
  displayName: "",
  ownerLogin: "",
  ownerType: "Organization" as GitHubAppOwnerType,
  publicBaseUrl: "",
  visibility: defaultVisibility,
  appId: "",
  clientId: "",
  clientSecret: "",
  privateKey: "",
  webhookSecret: "",
  slug: "",
};

function clearImportSecrets(values: GitHubAppImportValues): GitHubAppImportValues {
  return { ...values, clientSecret: "", privateKey: "", webhookSecret: "" };
}

function importSettingsUrl(values: GitHubAppImportValues) {
  if (!values.ownerLogin.trim() || !values.slug.trim()) return undefined;
  return githubAppSettingsURL(values.ownerType, values.ownerLogin, values.slug);
}

function showError(error: unknown, fallback: string, toast: ReturnType<typeof useToast>["toast"]) {
  const detail = appRegistrationError(error);
  toast({ description: detail?.error ?? fallback, variant: "error" });
}

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <Label className="block space-y-1.5">
      <span>{label}</span>
      {children}
      {error && <span className="block text-xs font-normal text-destructive">{error}</span>}
    </Label>
  );
}
