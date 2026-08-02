"use client";

import { useEffect, useState } from "react";
import { IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import type { StartGitHubAppManifestResponse } from "@/lib/types/github";
import {
  appRegistrationError,
  defaultVisibility,
  submitManifestToGitHub,
  validateRegistrationBasics,
  type AppSetupErrors,
} from "./github-app-onboarding-model";
import { GitHubAppPolicyDialog } from "./github-app-policy-dialog";
import { GitHubAppVisibilityField } from "./github-app-visibility-field";
import type { useGitHubAppRegistrations } from "@/hooks/domains/github/use-github-app-registrations";
import { useTranslation } from "react-i18next";

type RegistrationHook = ReturnType<typeof useGitHubAppRegistrations>;

export function GitHubAppCreateForm({
  workspaceId,
  registrations,
}: {
  workspaceId: string;
  registrations: RegistrationHook;
}) {
  const { t } = useTranslation();
  const [displayName, setDisplayName] = useState("");
  const [ownerType, setOwnerType] = useState<"user" | "organization">("organization");
  const [ownerLogin, setOwnerLogin] = useState("");
  const [publicBaseUrl, setPublicBaseUrl] = useState("");
  const [visibility, setVisibility] = useState(defaultVisibility);
  const [errors, setErrors] = useState<AppSetupErrors>({});
  const [handoff, setHandoff] = useState<StartGitHubAppManifestResponse | null>(null);
  const { toast } = useToast();

  useEffect(() => {
    setDisplayName("");
    setOwnerType("organization");
    setOwnerLogin("");
    setPublicBaseUrl("");
    setVisibility(defaultVisibility);
    setErrors({});
    setHandoff(null);
  }, [workspaceId]);

  async function prepare(event: React.FormEvent) {
    event.preventDefault();
    const basics = validateRegistrationBasics({
      displayName,
      ownerType,
      ownerLogin,
      publicBaseUrl,
    });
    setErrors(basics.errors);
    if (!basics.publicBaseUrl || Object.keys(basics.errors).length) return;
    try {
      setHandoff(
        await registrations.startManifest({
          display_name: basics.displayName,
          owner_type: ownerType,
          owner_login: basics.ownerLogin,
          visibility,
          public_base_url: basics.publicBaseUrl,
        }),
      );
    } catch (error) {
      const detail = appRegistrationError(error);
      toast({
        description: detail?.error ?? t("github:githubAppSetupCouldNotStart"),
        variant: "error",
      });
    }
  }

  if (handoff) {
    return <ManifestHandoff handoff={handoff} onEdit={() => setHandoff(null)} />;
  }

  return (
    <CreateAppFields
      displayName={displayName}
      ownerType={ownerType}
      ownerLogin={ownerLogin}
      publicBaseUrl={publicBaseUrl}
      visibility={visibility}
      errors={errors}
      mutating={registrations.mutating}
      onDisplayName={setDisplayName}
      onOwnerType={setOwnerType}
      onOwnerLogin={setOwnerLogin}
      onPublicBaseUrl={setPublicBaseUrl}
      onVisibility={setVisibility}
      onSubmit={prepare}
    />
  );
}

function ManifestHandoff({
  handoff,
  onEdit,
}: {
  handoff: StartGitHubAppManifestResponse;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:githubIsReadyToCreateThe")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:githubWillShowTheGeneratedPermissions")}
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          className="h-11 cursor-pointer"
          onClick={() => submitManifestToGitHub(handoff.registration_url, handoff.manifest)}
        >
          {t("github:continueOnGithub")}
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={onEdit}>
          {t("github:editDetails")}
        </Button>
      </div>
    </div>
  );
}

type CreateFieldsProps = {
  displayName: string;
  ownerType: "user" | "organization";
  ownerLogin: string;
  publicBaseUrl: string;
  visibility: "private" | "public";
  errors: AppSetupErrors;
  mutating: boolean;
  onDisplayName: (value: string) => void;
  onOwnerType: (value: "user" | "organization") => void;
  onOwnerLogin: (value: string) => void;
  onPublicBaseUrl: (value: string) => void;
  onVisibility: (value: "private" | "public") => void;
  onSubmit: (event: React.FormEvent) => void;
};

function CreateAppFields(props: CreateFieldsProps) {
  const { t } = useTranslation();
  return (
    <form onSubmit={props.onSubmit} className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:createAGithubApp")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:kandevGeneratesTheExactAppPolicy")}
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("github:nameInKandev")} error={props.errors.displayName}>
          <Input
            className="h-11"
            value={props.displayName}
            onChange={(event) => props.onDisplayName(event.target.value)}
          />
        </Field>
        <Field label={t("github:githubOwnerLogin")} error={props.errors.ownerLogin}>
          <Input
            className="h-11"
            value={props.ownerLogin}
            onChange={(event) => props.onOwnerLogin(event.target.value)}
          />
        </Field>
      </div>
      <RadioGroup
        value={props.ownerType}
        onValueChange={(value) => props.onOwnerType(value as CreateFieldsProps["ownerType"])}
        className="flex min-h-11 flex-wrap items-center gap-5"
      >
        <Label className="flex cursor-pointer items-center gap-2">
          <RadioGroupItem value="organization" /> {t("github:organization")}
        </Label>
        <Label className="flex cursor-pointer items-center gap-2">
          <RadioGroupItem value="user" /> {t("github:personalAccount")}
        </Label>
      </RadioGroup>
      <Field label={t("github:publicKandevUrl")} error={props.errors.publicBaseUrl}>
        <Input
          className="h-11"
          type="url"
          placeholder="https://kandev.example.com"
          value={props.publicBaseUrl}
          onChange={(event) => props.onPublicBaseUrl(event.target.value)}
        />
      </Field>
      <p className="text-xs text-muted-foreground">{t("github:githubMustReachThisHttpsOrigin")}</p>
      <GitHubAppVisibilityField value={props.visibility} onChange={props.onVisibility} />
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button type="submit" disabled={props.mutating} className="h-11 cursor-pointer">
          {props.mutating && <Spinner className="mr-2 h-4 w-4" />}
          {t("github:prepareAppOnGithub")}
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
        <GitHubAppPolicyDialog />
      </div>
    </form>
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
