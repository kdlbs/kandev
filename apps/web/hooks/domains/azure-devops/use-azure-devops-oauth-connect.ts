"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useToast } from "@/components/toast-provider";
import {
  completeAzureDevOpsOAuth,
  startAzureDevOpsOAuth,
} from "@/lib/api/domains/azure-devops-oauth-api";
import type { AzureDevOpsConfig, StartAzureDevOpsOAuthRequest } from "@/lib/types/azure-devops";

// i18n-exempt: wire discriminant posted by the backend callback page, never shown.
const CALLBACK_MESSAGE_TYPE = "azure-devops-oauth-callback";
// i18n-exempt: OAuth error code that Microsoft returns when the user declines.
const ACCESS_DENIED = "access_denied";

type CallbackMessage = { type: string; code: string; state: string; error: string };

function isCallbackMessage(value: unknown): value is CallbackMessage {
  if (!value || typeof value !== "object") return false;
  const message = value as Partial<CallbackMessage>;
  return (
    message.type === CALLBACK_MESSAGE_TYPE &&
    typeof message.code === "string" &&
    typeof message.state === "string" &&
    typeof message.error === "string"
  );
}

/** Reads code and state from the address Microsoft redirected to. */
export function parseOAuthRedirect(value: string): { code: string; state: string } | null {
  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    return null;
  }
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");
  return code && state ? { code, state } : null;
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function useLatestRef<T>(value: T) {
  const ref = useRef(value);
  useEffect(() => {
    ref.current = value;
  }, [value]);
  return ref;
}

function usePopupMessages(
  popupRef: RefObject<Window | null>,
  pendingStateRef: RefObject<string | null>,
  onMessage: (message: CallbackMessage) => void,
) {
  useEffect(() => {
    const handleMessage = (event: MessageEvent<unknown>) => {
      if (event.origin !== window.location.origin || event.source !== popupRef.current) return;
      if (!isCallbackMessage(event.data) || event.data.state !== pendingStateRef.current) return;
      onMessage(event.data);
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [onMessage, pendingStateRef, popupRef]);
  useEffect(() => () => popupRef.current?.close(), [popupRef]);
}

const POPUP_WATCH_INTERVAL_MS = 250;

/** Reads the authorization result once the popup is back on Kandev's origin.
 *  While it shows Microsoft's pages, reading its location throws. */
function readSameOriginRedirect(popup: Window): CallbackMessage | null {
  try {
    if (popup.location.origin !== window.location.origin) return null;
    const params = new URLSearchParams(popup.location.search);
    const state = params.get("state");
    if (!state) return null;
    return {
      type: CALLBACK_MESSAGE_TYPE,
      code: params.get("code") ?? "",
      state,
      error: params.get("error") ?? "",
    };
  } catch {
    return null;
  }
}

/** Completes sign-in when the redirect lands on any Kandev page, such as an
 *  operator-configured redirect URL without the callback path. */
function usePopupRedirectWatch(
  active: boolean,
  popupRef: RefObject<Window | null>,
  pendingStateRef: RefObject<string | null>,
  onMessage: (message: CallbackMessage) => void,
) {
  useEffect(() => {
    if (!active) return;
    const interval = window.setInterval(() => {
      const popup = popupRef.current;
      // A closed popup can no longer report a result; the paste fallback
      // stays available for an address the user copied before closing it.
      if (!popup || popup.closed) {
        window.clearInterval(interval);
        return;
      }
      const message = readSameOriginRedirect(popup);
      if (message && message.state === pendingStateRef.current) onMessage(message);
    }, POPUP_WATCH_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [active, onMessage, pendingStateRef, popupRef]);
}

export type AzureDevOpsOAuthConnect = {
  connecting: boolean;
  completing: boolean;
  pasteOpen: boolean;
  pasteUrl: string;
  setPasteUrl: (value: string) => void;
  connect: () => void;
  completeFromPaste: () => void;
  cancel: () => void;
};

/** Completes one authorization result at a time: the pasted address, the
 *  popup message, and the redirect watch can all report the same result. */
function useOAuthCompletion(
  workspaceId: string,
  pendingStateRef: RefObject<string | null>,
  reset: () => void,
  fail: (error: string) => void,
  onConnectedRef: RefObject<(config: AzureDevOpsConfig) => void>,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [completing, setCompleting] = useState(false);
  const completingRef = useRef(false);
  const complete = useCallback(
    async (code: string, state: string) => {
      if (completingRef.current) return;
      completingRef.current = true;
      pendingStateRef.current = null;
      setCompleting(true);
      try {
        const config = await completeAzureDevOpsOAuth(workspaceId, { code, state });
        reset();
        toast({ description: t("azuredevops:oauthConnected"), variant: "success" });
        onConnectedRef.current(config);
      } catch (err) {
        fail(errorMessage(err));
      } finally {
        completingRef.current = false;
        setCompleting(false);
      }
    },
    [fail, onConnectedRef, pendingStateRef, reset, t, toast, workspaceId],
  );
  return { completing, completingRef, complete };
}

export function useAzureDevOpsOAuthConnect(
  workspaceId: string,
  request: StartAzureDevOpsOAuthRequest,
  onConnected: (config: AzureDevOpsConfig) => void,
): AzureDevOpsOAuthConnect {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [connecting, setConnecting] = useState(false);
  const [pasteOpen, setPasteOpen] = useState(false);
  const [pasteUrl, setPasteUrl] = useState("");
  const popupRef = useRef<Window | null>(null);
  const pendingStateRef = useRef<string | null>(null);
  const onConnectedRef = useLatestRef(onConnected);
  const requestRef = useLatestRef(request);

  const fail = useCallback(
    (error: string) =>
      toast({ description: t("azuredevops:oauthFailed", { error }), variant: "error" }),
    [t, toast],
  );

  const reset = useCallback(() => {
    popupRef.current?.close();
    popupRef.current = null;
    pendingStateRef.current = null;
    setPasteOpen(false);
    setPasteUrl("");
  }, []);

  const { completing, completingRef, complete } = useOAuthCompletion(
    workspaceId,
    pendingStateRef,
    reset,
    fail,
    onConnectedRef,
  );

  const handleMessage = useCallback(
    (message: CallbackMessage) => {
      // Both the callback page and the redirect watch can report one result.
      pendingStateRef.current = null;
      if (!message.error) {
        void complete(message.code, message.state);
        return;
      }
      reset();
      if (message.error === ACCESS_DENIED) {
        toast({ description: t("azuredevops:oauthCancelled"), variant: "error" });
      } else {
        fail(message.error);
      }
    },
    [complete, fail, reset, t, toast],
  );
  usePopupMessages(popupRef, pendingStateRef, handleMessage);
  usePopupRedirectWatch(pasteOpen, popupRef, pendingStateRef, handleMessage);

  const connect = useCallback(async () => {
    if (completingRef.current) return;
    const popup = window.open("", "_blank");
    if (!popup) {
      toast({ description: t("azuredevops:oauthPopupBlocked"), variant: "error" });
      return;
    }
    popupRef.current = popup;
    setConnecting(true);
    try {
      const { authUrl, state } = await startAzureDevOpsOAuth(workspaceId, requestRef.current);
      pendingStateRef.current = state;
      popup.location.assign(authUrl);
      setPasteOpen(true);
    } catch (err) {
      reset();
      fail(errorMessage(err));
    } finally {
      setConnecting(false);
    }
  }, [fail, requestRef, reset, t, toast, workspaceId]);

  const completeFromPaste = useCallback(() => {
    const redirect = parseOAuthRedirect(pasteUrl);
    if (!redirect) {
      toast({ description: t("azuredevops:oauthPasteInvalid"), variant: "error" });
      return;
    }
    void complete(redirect.code, redirect.state);
  }, [complete, pasteUrl, t, toast]);

  return {
    connecting,
    completing,
    pasteOpen,
    pasteUrl,
    setPasteUrl,
    connect: () => void connect(),
    completeFromPaste,
    cancel: reset,
  };
}
