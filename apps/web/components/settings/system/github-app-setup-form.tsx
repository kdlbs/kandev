"use client";

import { useState } from "react";
import { IconBrandGithub, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Spinner } from "@kandev/ui/spinner";

import type { StartDeploymentGitHubAppRequest } from "@/lib/types/github";
import {
  type DeploymentAppOwnerChoice,
  type DeploymentAppSetupErrors,
  validateDeploymentAppSetup,
} from "./github-app-settings-model";
import { GitHubAppPermissionsDialog } from "./github-app-permissions-dialog";

export function GitHubAppSetupForm({
  busy,
  onStart,
}: {
  busy: boolean;
  onStart: (request: StartDeploymentGitHubAppRequest) => Promise<void>;
}) {
  const [ownerType, setOwnerType] = useState<DeploymentAppOwnerChoice>("organization");
  const [ownerLogin, setOwnerLogin] = useState("");
  const [publicBaseUrl, setPublicBaseUrl] = useState("");
  const [errors, setErrors] = useState<DeploymentAppSetupErrors>({});

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const validation = validateDeploymentAppSetup({ ownerType, ownerLogin, publicBaseUrl });
    setErrors(validation.errors);
    if (!validation.values) return;
    await onStart(validation.values);
  };

  return (
    <form className="space-y-5" onSubmit={submit} data-testid="github-app-setup-form">
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">App owner</legend>
        <p className="text-xs text-muted-foreground">
          Choose the GitHub account that will own the deployment App registration.
        </p>
        <RadioGroup
          value={ownerType}
          onValueChange={(value) => setOwnerType(value as DeploymentAppOwnerChoice)}
          className="grid gap-2 sm:grid-cols-2"
          data-testid="github-app-owner-type"
        >
          <OwnerChoice
            value="organization"
            title="Organization"
            description="Recommended for company-managed automation."
          />
          <OwnerChoice
            value="user"
            title="Personal account"
            description="Useful for a personal self-hosted deployment."
          />
        </RadioGroup>
      </fieldset>

      <div className="space-y-2">
        <Label htmlFor="github-app-owner-login">
          {ownerType === "organization" ? "Organization login" : "GitHub username"}
        </Label>
        <Input
          id="github-app-owner-login"
          value={ownerLogin}
          onChange={(event) => setOwnerLogin(event.target.value)}
          placeholder={ownerType === "organization" ? "acme" : "octocat"}
          className="h-11"
          aria-invalid={Boolean(errors.ownerLogin)}
          aria-describedby={errors.ownerLogin ? "github-app-owner-error" : undefined}
          aria-errormessage={errors.ownerLogin ? "github-app-owner-error" : undefined}
        />
        {errors.ownerLogin && (
          <p id="github-app-owner-error" className="text-xs text-destructive">
            {errors.ownerLogin}
          </p>
        )}
      </div>

      <PublicBaseUrlField
        value={publicBaseUrl}
        error={errors.publicBaseUrl}
        onChange={setPublicBaseUrl}
      />

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <Button
          type="submit"
          disabled={busy}
          className="min-h-11 w-full cursor-pointer sm:w-auto"
          data-testid="github-app-create-button"
        >
          {busy ? (
            <Spinner className="mr-2 h-4 w-4" />
          ) : (
            <IconBrandGithub className="mr-2 h-4 w-4" />
          )}
          Create on GitHub
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
        <GitHubAppPermissionsDialog />
      </div>
    </form>
  );
}

function PublicBaseUrlField({
  value,
  error,
  onChange,
}: {
  value: string;
  error?: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor="github-app-public-url">Public Kandev URL</Label>
      <Input
        id="github-app-public-url"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="https://kandev.example.com"
        inputMode="url"
        className="h-11"
        aria-invalid={Boolean(error)}
        aria-describedby={
          error
            ? "github-app-public-url-help github-app-public-url-error"
            : "github-app-public-url-help"
        }
        aria-errormessage={error ? "github-app-public-url-error" : undefined}
      />
      <p id="github-app-public-url-help" className="text-xs text-muted-foreground">
        GitHub must reach this HTTPS origin for callbacks and signed webhooks. Localhost and
        private-network addresses are not supported; use a reverse proxy or HTTPS tunnel.
      </p>
      {error && (
        <p id="github-app-public-url-error" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

function OwnerChoice({
  value,
  title,
  description,
}: {
  value: DeploymentAppOwnerChoice;
  title: string;
  description: string;
}) {
  return (
    <Label className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border p-3 has-data-[state=checked]:border-primary has-data-[state=checked]:bg-muted/50">
      <RadioGroupItem value={value} className="mt-0.5" />
      <span className="min-w-0">
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs font-normal text-muted-foreground">{description}</span>
      </span>
    </Label>
  );
}
