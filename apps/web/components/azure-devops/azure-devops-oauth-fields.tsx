"use client";

import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconBrandWindows,
  IconCircleCheck,
  IconLogout,
} from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";

import type {
  AzureDevOpsAuthMethod,
  AzureDevOpsConfig,
  StartAzureDevOpsOAuthRequest,
} from "@/lib/types/azure-devops";

import {
  useAzureDevOpsOAuthConnect,
  type AzureDevOpsOAuthConnect,
} from "@/hooks/domains/azure-devops/use-azure-devops-oauth-connect";

export function AzureDevOpsAuthMethodField({
  value,
  onChange,
  disabled,
}: {
  value: AzureDevOpsAuthMethod;
  onChange: (value: AzureDevOpsAuthMethod) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5 sm:col-span-2">
      <Label htmlFor="azure-devops-auth-method">{t("azuredevops:authMethod")}</Label>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as AzureDevOpsAuthMethod)}
        disabled={disabled}
      >
        <SelectTrigger
          id="azure-devops-auth-method"
          className="w-full"
          data-testid="azure-devops-auth-method"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="pat">{t("azuredevops:personalAccessToken")}</SelectItem>
          <SelectItem value="oauth" data-testid="azure-devops-auth-oauth">
            {t("azuredevops:authMethodMicrosoft")}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

/** True when the next sign-in would use a different application than the
 *  one the connection uses now. */
function applicationChangePending(clientId: string, defaultClientId: string, appInUse?: string) {
  const next = clientId.trim() || defaultClientId;
  return !!appInUse && !!next && next.toLowerCase() !== appInUse.toLowerCase();
}

function OAuthClientIdField({
  clientId,
  defaultClientId,
  appInUse,
  disabled,
  onChange,
}: {
  clientId: string;
  defaultClientId: string;
  appInUse?: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const missingClient = !clientId.trim() && !defaultClientId;
  const changePending = applicationChangePending(clientId, defaultClientId, appInUse);
  return (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor="azure-devops-oauth-client-id">{t("azuredevops:oauthClientId")}</Label>
      <Input
        id="azure-devops-oauth-client-id"
        value={clientId}
        onChange={(event) => onChange(event.target.value)}
        placeholder={defaultClientId}
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
        className={controlSizingClassName("standard", "font-mono")}
        data-testid="azure-devops-oauth-client-id"
      />
      <p
        className={missingClient ? "text-xs text-destructive" : "text-xs text-muted-foreground"}
        data-testid={missingClient ? "azure-devops-oauth-client-required" : undefined}
      >
        {missingClient
          ? t("azuredevops:oauthClientIdRequired")
          : t("azuredevops:oauthApplicationHelp")}
      </p>
      {changePending && (
        <p
          className="text-xs text-amber-600 dark:text-amber-400"
          role="status"
          data-testid="azure-devops-oauth-app-pending"
        >
          {t("azuredevops:oauthApplicationChangePending")}
        </p>
      )}
    </div>
  );
}

function OAuthPastePanel({ oauth }: { oauth: AzureDevOpsOAuthConnect }) {
  const { t } = useTranslation();
  return (
    <Alert data-testid="azure-devops-oauth-paste">
      <AlertDescription className="space-y-3">
        <p className="font-medium">{t("azuredevops:oauthPasteTitle")}</p>
        <p className="text-xs">{t("azuredevops:oauthPasteHelp")}</p>
        <Input
          type="url"
          value={oauth.pasteUrl}
          onChange={(event) => oauth.setPasteUrl(event.target.value)}
          disabled={oauth.completing}
          aria-label={t("azuredevops:oauthPasteTitle")}
          autoComplete="off"
          className={controlSizingClassName("standard", "min-w-0")}
          data-testid="azure-devops-oauth-paste-url"
        />
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            type="button"
            onClick={oauth.completeFromPaste}
            disabled={oauth.completing || !oauth.pasteUrl}
            className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
            data-testid="azure-devops-oauth-paste-complete"
          >
            {t("azuredevops:oauthPasteComplete")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={oauth.cancel}
            className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
            data-testid="azure-devops-oauth-paste-cancel"
          >
            {t("common:cancel")}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}

function OAuthStatus({
  config,
  signedIn,
}: {
  config: AzureDevOpsConfig | null;
  signedIn: boolean;
}) {
  const { t } = useTranslation();
  if (signedIn && config?.reauthRequired) {
    return (
      <p
        className="flex items-start gap-2 text-sm text-destructive"
        role="status"
        data-testid="azure-devops-oauth-reauth"
      >
        <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
        {t("azuredevops:oauthReauthRequired")}
      </p>
    );
  }
  if (signedIn) {
    return (
      <div className="min-w-0 space-y-1">
        <p className="flex items-center gap-2 text-sm" data-testid="azure-devops-oauth-account">
          <IconCircleCheck className="size-4 shrink-0 text-emerald-600" />
          {config?.oauthAccount
            ? t("azuredevops:oauthSignedInAs", { name: config.oauthAccount })
            : t("azuredevops:oauthSignedIn")}
        </p>
        {config?.oauthClientId && (
          <p
            className="break-all pl-6 text-xs text-muted-foreground"
            data-testid="azure-devops-oauth-app-in-use"
          >
            {t("azuredevops:oauthApplicationInUse", { clientId: config.oauthClientId })}
          </p>
        )}
      </div>
    );
  }
  return <p className="text-sm text-muted-foreground">{t("azuredevops:oauthDescription")}</p>;
}

function signInLabel(
  t: (key: string) => string,
  signedIn: boolean,
  config: AzureDevOpsConfig | null,
): string {
  if (signedIn && !config?.reauthRequired) return t("azuredevops:oauthSignInAgain");
  return t("azuredevops:oauthSignIn");
}

/** Localized progress for the polite status region; empty when idle. */
function progressText(
  t: (key: string) => string,
  oauth: AzureDevOpsOAuthConnect,
  signingOut: boolean,
): string {
  if (oauth.connecting) return t("azuredevops:oauthSigningIn");
  if (oauth.completing) return t("azuredevops:oauthCompleting");
  if (signingOut) return t("azuredevops:oauthSigningOut");
  return "";
}

function OAuthActions({
  oauth,
  config,
  signedIn,
  canSignIn,
  disabled,
  signingOut,
  onSignOut,
}: {
  oauth: AzureDevOpsOAuthConnect;
  config: AzureDevOpsConfig | null;
  signedIn: boolean;
  canSignIn: boolean;
  disabled: boolean;
  signingOut: boolean;
  onSignOut: () => void;
}) {
  const { t } = useTranslation();
  const secondary = signedIn && !config?.reauthRequired;
  return (
    <div className="flex w-full shrink-0 flex-col gap-2 sm:w-auto sm:flex-row">
      <Button
        type="button"
        variant={secondary ? "outline" : "default"}
        onClick={oauth.connect}
        disabled={!canSignIn || oauth.connecting || oauth.completing}
        className={controlSizingClassName("standard", "w-full shrink-0 cursor-pointer sm:w-auto")}
        data-testid="azure-devops-oauth-connect"
      >
        <IconBrandWindows className="h-4 w-4" />
        {signInLabel(t, signedIn, config)}
      </Button>
      {signedIn && (
        <Button
          type="button"
          variant="ghost"
          onClick={onSignOut}
          disabled={disabled || signingOut || oauth.connecting}
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          data-testid="azure-devops-oauth-sign-out"
        >
          <IconLogout className="h-4 w-4" />
          {t("azuredevops:oauthSignOut")}
        </Button>
      )}
    </div>
  );
}

/** Microsoft account block: signed-in identity, sign-in action, and paste fallback. */
export function AzureDevOpsOAuthFields({
  workspaceId,
  config,
  request,
  clientId,
  defaultClientId,
  onClientIdChange,
  onSignOut,
  signingOut,
  signedIn,
  organizationValid,
  disabled,
  onConnected,
}: {
  workspaceId: string;
  config: AzureDevOpsConfig | null;
  request: StartAzureDevOpsOAuthRequest;
  clientId: string;
  defaultClientId: string;
  onClientIdChange: (value: string) => void;
  onSignOut: () => void;
  signingOut: boolean;
  signedIn: boolean;
  organizationValid: boolean;
  disabled: boolean;
  onConnected: (config: AzureDevOpsConfig) => void;
}) {
  const { t } = useTranslation();
  const oauth = useAzureDevOpsOAuthConnect(workspaceId, request, onConnected);
  const hasApplication = !!(clientId.trim() || defaultClientId);
  return (
    <div className="space-y-3 rounded-md border p-3 sm:col-span-2" data-testid="azure-devops-oauth">
      <span role="status" className="sr-only" data-testid="azure-devops-oauth-progress">
        {progressText(t, oauth, signingOut)}
      </span>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <OAuthStatus config={config} signedIn={signedIn} />
        <OAuthActions
          oauth={oauth}
          config={config}
          signedIn={signedIn}
          canSignIn={!disabled && organizationValid && hasApplication}
          disabled={disabled}
          signingOut={signingOut}
          onSignOut={onSignOut}
        />
      </div>
      <OAuthClientIdField
        clientId={clientId}
        defaultClientId={defaultClientId}
        appInUse={signedIn ? config?.oauthClientId : undefined}
        disabled={disabled || oauth.connecting}
        onChange={onClientIdChange}
      />
      {oauth.pasteOpen && <OAuthPastePanel oauth={oauth} />}
    </div>
  );
}
