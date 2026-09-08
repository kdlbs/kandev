"use client";

import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";

import { useJiraOAuthConnect } from "./use-jira-oauth-connect";

type OAuthFieldsProps = {
  form: { siteUrl: string };
  loading: boolean;
  workspaceId: string;
  connected: boolean;
  tokenExpiresAt?: string | null;
  onConnected: () => void;
};

// Fallback for remote servers: the OAuth redirect targets localhost, so the user
// pastes the full callback URL back for the code+state exchange.
function OAuthPasteDialog({
  pasteUrl,
  setPasteUrl,
  completing,
  onComplete,
  onCancel,
}: {
  pasteUrl: string;
  setPasteUrl: (value: string) => void;
  completing: boolean;
  onComplete: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Alert>
      <AlertDescription className="space-y-3">
        <p className="font-medium">{t("jira:oauthPasteInstructions")}</p>
        <p className="text-xs">{t("jira:oauthPasteHelp")}</p>
        <Input
          type="text"
          placeholder="http://localhost:38429/api/v1/jira/oauth/callback?code=...&state=..."
          value={pasteUrl}
          onChange={(e) => setPasteUrl(e.target.value)}
          disabled={completing}
          data-testid="jira-oauth-paste-url"
        />
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            type="button"
            size="sm"
            onClick={onComplete}
            disabled={completing || !pasteUrl}
            className="h-11 w-full cursor-pointer sm:w-auto"
          >
            {completing ? t("jira:connecting") : t("jira:oauthComplete")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onCancel}
            className="h-11 w-full cursor-pointer sm:w-auto"
          >
            {t("jira:oauthCancel")}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}

export function OAuthFields({
  form,
  loading,
  workspaceId,
  connected,
  tokenExpiresAt,
  onConnected,
}: OAuthFieldsProps) {
  const { t } = useTranslation();
  const oauth = useJiraOAuthConnect(workspaceId, form.siteUrl, onConnected);
  return (
    <div className="space-y-4">
      {connected && (
        <Alert>
          <AlertDescription>
            {t("jira:oauthConnected")}
            {tokenExpiresAt && (
              <span className="ml-2 text-xs text-muted-foreground">
                {t("jira:oauthTokenExpires", { date: new Date(tokenExpiresAt).toLocaleString() })}
              </span>
            )}
          </AlertDescription>
        </Alert>
      )}
      <Button
        type="button"
        onClick={oauth.handleConnect}
        disabled={oauth.connecting || loading || !form.siteUrl}
        className="h-11 w-full cursor-pointer sm:w-auto"
        data-testid="jira-oauth-connect"
      >
        {oauth.connecting ? t("jira:connecting") : t("jira:connectWithAtlassian")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("jira:oauthDescription")}</p>
      {oauth.showPasteDialog && (
        <OAuthPasteDialog
          pasteUrl={oauth.pasteUrl}
          setPasteUrl={oauth.setPasteUrl}
          completing={oauth.completing}
          onComplete={oauth.handlePasteComplete}
          onCancel={oauth.cancelPaste}
        />
      )}
    </div>
  );
}
