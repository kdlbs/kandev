"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useToast } from "@/components/toast-provider";
import { completeJiraOAuth, startJiraOAuth } from "@/lib/api/domains/jira-api";

export type OAuthConnectState = {
  connecting: boolean;
  completing: boolean;
  showPasteDialog: boolean;
  pasteUrl: string;
  setPasteUrl: (value: string) => void;
  handleConnect: () => void;
  handlePasteComplete: () => void;
  cancelPaste: () => void;
};

type JiraOAuthCallbackMessage = {
  type: "jira-oauth-callback";
  code: string;
  state: string;
};

function isJiraOAuthCallbackMessage(value: unknown): value is JiraOAuthCallbackMessage {
  if (!value || typeof value !== "object") return false;
  const message = value as Partial<JiraOAuthCallbackMessage>;
  return (
    message.type === "jira-oauth-callback" &&
    typeof message.code === "string" &&
    message.code.length > 0 &&
    typeof message.state === "string" &&
    message.state.length > 0
  );
}

function useOAuthPopupMessage(
  popupRef: RefObject<Window | null>,
  pendingStateRef: RefObject<string | null>,
  completeOAuth: (code: string, state: string) => Promise<void>,
) {
  useEffect(() => {
    const handleMessage = (event: MessageEvent<unknown>) => {
      if (event.origin !== window.location.origin || event.source !== popupRef.current) return;
      if (!isJiraOAuthCallbackMessage(event.data)) return;
      if (event.data.state !== pendingStateRef.current) return;
      void completeOAuth(event.data.code, event.data.state);
    };
    window.addEventListener("message", handleMessage);
    return () => {
      window.removeEventListener("message", handleMessage);
      popupRef.current?.close();
    };
  }, [completeOAuth, pendingStateRef, popupRef]);
}

function useLatestRef<T>(value: T) {
  const ref = useRef(value);
  useEffect(() => {
    ref.current = value;
  }, [value]);
  return ref;
}

export function useJiraOAuthConnect(
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
  const popupRef = useRef<Window | null>(null);
  const pendingStateRef = useRef<string | null>(null);
  const onConnectedRef = useLatestRef(onConnected);

  const showError = useCallback(
    (error: string) => {
      toast({ description: t("jira:oauthStartFailed", { error }), variant: "error" });
    },
    [t, toast],
  );

  const completeOAuth = useCallback(
    async (code: string, state: string) => {
      setCompleting(true);
      try {
        await completeJiraOAuth(code, state);
        toast({ description: t("jira:oauthConnected"), variant: "success" });
        setShowPasteDialog(false);
        setPasteUrl("");
        pendingStateRef.current = null;
        popupRef.current?.close();
        popupRef.current = null;
        onConnectedRef.current();
      } catch (err) {
        showError(err instanceof Error ? err.message : String(err));
        setShowPasteDialog(true);
      } finally {
        setCompleting(false);
      }
    },
    [t, toast, showError],
  );

  useOAuthPopupMessage(popupRef, pendingStateRef, completeOAuth);

  const handleConnect = useCallback(async () => {
    const popup = window.open("", "_blank");
    if (!popup) {
      showError(t("jira:oauthPopupBlocked"));
      return;
    }
    popupRef.current = popup;
    setConnecting(true);
    try {
      const { authUrl } = await startJiraOAuth(siteUrl, { workspaceId });
      const state = new URL(authUrl).searchParams.get("state");
      if (!state) throw new Error(t("jira:oauthPasteInvalid"));
      pendingStateRef.current = state;
      popup.location.assign(authUrl);
      setShowPasteDialog(true);
    } catch (err) {
      popup.close();
      popupRef.current = null;
      pendingStateRef.current = null;
      showError(err instanceof Error ? err.message : String(err));
    } finally {
      setConnecting(false);
    }
  }, [workspaceId, siteUrl, showError, t]);

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
    popupRef.current?.close();
    popupRef.current = null;
    pendingStateRef.current = null;
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
