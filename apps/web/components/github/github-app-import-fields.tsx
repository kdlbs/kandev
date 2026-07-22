import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Textarea } from "@kandev/ui/textarea";
import type { GitHubAppOwnerType, GitHubAppVisibility } from "@/lib/types/github";
import type { AppSetupErrors } from "./github-app-onboarding-model";

export type GitHubAppImportValues = {
  displayName: string;
  ownerLogin: string;
  ownerType: GitHubAppOwnerType;
  publicBaseUrl: string;
  visibility: GitHubAppVisibility;
  appId: string;
  clientId: string;
  clientSecret: string;
  privateKey: string;
  webhookSecret: string;
  slug: string;
};

type Update = (name: keyof GitHubAppImportValues, value: string) => void;

export function GitHubAppImportIdentityFields({
  values,
  errors,
  update,
}: {
  values: GitHubAppImportValues;
  errors: AppSetupErrors;
  update: Update;
}) {
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Name in Kandev" error={errors.displayName}>
          <Input
            className="h-11"
            value={values.displayName}
            onChange={(e) => update("displayName", e.target.value)}
          />
        </Field>
        <Field label="GitHub App slug" error={errors.slug}>
          <Input
            className="h-11"
            value={values.slug}
            onChange={(e) => update("slug", e.target.value)}
          />
        </Field>
        <Field label="GitHub owner login" error={errors.ownerLogin}>
          <Input
            className="h-11"
            value={values.ownerLogin}
            onChange={(e) => update("ownerLogin", e.target.value)}
          />
        </Field>
      </div>
      <RadioGroup
        value={values.ownerType}
        onValueChange={(value) => update("ownerType", value)}
        className="flex min-h-11 flex-wrap items-center gap-5"
      >
        <Label className="flex cursor-pointer items-center gap-2">
          <RadioGroupItem value="Organization" /> Organization
        </Label>
        <Label className="flex cursor-pointer items-center gap-2">
          <RadioGroupItem value="User" /> Personal account
        </Label>
      </RadioGroup>
    </div>
  );
}

export function GitHubAppImportSecretFields({
  values,
  errors,
  update,
}: {
  values: GitHubAppImportValues;
  errors: AppSetupErrors;
  update: Update;
}) {
  const labels = {
    appId: "App ID",
    clientId: "Client ID",
    clientSecret: "Client secret",
    webhookSecret: "Webhook secret",
  } as const;
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {(Object.keys(labels) as (keyof typeof labels)[]).map((name) => (
        <Field key={name} label={labels[name]} error={errors[name]}>
          <Input
            className="h-11"
            type={name.includes("Secret") || name === "webhookSecret" ? "password" : "text"}
            autoComplete="off"
            value={values[name]}
            onChange={(event) => update(name, event.target.value)}
          />
        </Field>
      ))}
      <div className="sm:col-span-2">
        <Field label="Private key (.pem)" error={errors.privateKey}>
          <Textarea
            className="min-h-28 font-mono text-xs"
            value={values.privateKey}
            onChange={(event) => update("privateKey", event.target.value)}
          />
        </Field>
      </div>
    </div>
  );
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
