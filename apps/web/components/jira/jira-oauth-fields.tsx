"use client";

import { useCallback, useState } from "react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { startJiraOAuth, completeJiraOAuth } from "@/lib/api/domains/jira-api";

type OAuthFieldsProps = {
  form: { siteUrl: string };
  loading: boolean;
  workspaceId: string;
  connected: boolean;
  tokenExpiresAt?: string | null;
  onConnected: () => void;
};

type OAuthConnectState = {
  connecting: boolean;
  completing: boolean;
  showPasteDialog: boolean;
  pasteUrl: string;
  setPasteUrl: (value: string) => void;
  handleConnect: () => void;
  handlePasteComplete: () => void;
  cancelPaste: () => void;
};

// Drives the OAuth connect + paste-URL exchange. Kept as a hook so OAuthFields
// stays JSX-only and both stay under the function-length limit.
function useOAuthConnect(
  workspaceId: string,
  siteUrl: string,
  onConnected: () => void,
): OAuthConnectState {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [connecting, setConnecting] = useState(false);
  const [completing, setCompleting] = useState(false);
  const [showPasteDialog, setShowPasteDialog] = useState(false);
  const [pasteUrl, setPasteUrl] = useState("");

  const showError = useCallback(
    (error: string) => {
      toast({ description: t("jira:oauthStartFailed", { error }), variant: "error" });
    },
    [t, toast],
  );

  const handleConnect = useCallback(async () => {
    setConnecting(true);
    try {
      const { authUrl } = await startJiraOAuth(siteUrl, { workspaceId });
      window.open(authUrl, "_blank");
      setShowPasteDialog(true);
    } catch (err) {
      showError(err instanceof Error ? err.message : String(err));
    } finally {
      setConnecting(false);
    }
  }, [workspaceId, siteUrl, showError]);

  const completeOAuth = useCallback(
    async (code: string, state: string) => {
      setCompleting(true);
      try {
        await completeJiraOAuth(code, state);
        toast({ description: t("jira:oauthConnected"), variant: "success" });
        setShowPasteDialog(false);
        setPasteUrl("");
        onConnected();
      } catch (err) {
        showError(err instanceof Error ? err.message : String(err));
        setShowPasteDialog(true);
      } finally {
        setCompleting(false);
      }
    },
    [t, toast, onConnected, showError],
  );

  const handlePasteComplete = useCallback(async () => {
    try {
      const url = new URL(pasteUrl);
      const code = url.searchParams.get("code");
      const state = url.searchParams.get("state");
      if (!code || !state) {
        toast({ description: t("jira:oauthPasteInvalid"), variant: "error" });
        return;
      }
      await completeOAuth(code, state);
    } catch {
      toast({ description: t("jira:oauthPasteInvalid"), variant: "error" });
    }
  }, [pasteUrl, t, toast, completeOAuth]);

  const cancelPaste = useCallback(() => {
    setShowPasteDialog(false);
    setPasteUrl("");
  }, []);

  return {
    connecting,
    completing,
    showPasteDialog,
    pasteUrl,
    setPasteUrl,
    handleConnect: () => void handleConnect(),
    handlePasteComplete: () => void handlePasteComplete(),
    cancelPaste,
  };
}

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
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            onClick={onComplete}
            disabled={completing || !pasteUrl}
            className="cursor-pointer"
          >
            {completing ? t("jira:connecting") : t("jira:oauthComplete")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onCancel}
            className="cursor-pointer"
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
  const oauth = useOAuthConnect(workspaceId, form.siteUrl, onConnected);
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
        className="cursor-pointer"
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
