"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  deletePushSubscription,
  getPushPublicKey,
  listPushSubscriptions,
  savePushSubscription,
  sendTestPush,
} from "@/lib/api/domains/push-api";
import { ApiError } from "@/lib/api/client";
import {
  getPushRegistration,
  isPushSupported,
  isSessionExpiredError,
  urlBase64ToUint8Array,
} from "@/lib/pwa/push-client";

export type WebPushStatus =
  | "loading"
  | "unsupported"
  | "denied"
  | "unsubscribed"
  | "subscribed"
  | "session-expired"
  | "error";

export type WebPushTestResult = "idle" | "sent" | "no-subscription" | "failed";

function toPayload(subscription: PushSubscription) {
  const json = subscription.toJSON();
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh: json.keys?.p256dh ?? "", auth: json.keys?.auth ?? "" },
  };
}

/** Reads this device's state: permission, browser subscription, server record. */
async function detectStatus(): Promise<WebPushStatus> {
  if (!isPushSupported()) return "unsupported";
  if (Notification.permission === "denied") return "denied";
  try {
    const registration = await getPushRegistration();
    const subscription = await registration.pushManager.getSubscription();
    if (!subscription) return "unsubscribed";
    const { subscriptions } = await listPushSubscriptions();
    if (!subscriptions.some((item) => item.endpoint === subscription.endpoint)) {
      // The browser still holds a subscription the server forgot (for example
      // after a database reset); re-register it instead of showing a dead toggle.
      await savePushSubscription(toPayload(subscription));
    }
    return "subscribed";
  } catch (error) {
    return isSessionExpiredError(error) ? "session-expired" : "error";
  }
}

/**
 * Subscribes this browser. Must be called from a user gesture: the permission
 * prompt is not shown otherwise, and Chrome may auto-block pages that ask on
 * load.
 */
async function subscribeDevice(): Promise<WebPushStatus> {
  if (!isPushSupported()) return "unsupported";
  const permission = await Notification.requestPermission();
  if (permission !== "granted") return permission === "denied" ? "denied" : "unsubscribed";
  const registration = await getPushRegistration();
  const { public_key: publicKey } = await getPushPublicKey();
  const existing = await registration.pushManager.getSubscription();
  // A subscription bound to a different application server key cannot be reused.
  if (existing) await existing.unsubscribe();
  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(publicKey),
  });
  try {
    await savePushSubscription(toPayload(subscription));
  } catch (error) {
    await subscription.unsubscribe().catch(() => undefined);
    throw error;
  }
  return "subscribed";
}

async function unsubscribeDevice(): Promise<void> {
  const registration = await getPushRegistration();
  const subscription = await registration.pushManager.getSubscription();
  if (!subscription) return;
  const { endpoint } = subscription;
  await subscription.unsubscribe();
  try {
    await deletePushSubscription(endpoint);
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 404)) throw error;
  }
}

/** Per-device Web Push state and actions for the notification settings page. */
export function useWebPush() {
  const [status, setStatus] = useState<WebPushStatus>("loading");
  const [busy, setBusy] = useState(false);
  const [testResult, setTestResult] = useState<WebPushTestResult>("idle");
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    void detectStatus().then((next) => {
      if (mounted.current) setStatus(next);
    });
    return () => {
      mounted.current = false;
    };
  }, []);

  const run = useCallback(async (action: () => Promise<WebPushStatus | void>) => {
    setBusy(true);
    setTestResult("idle");
    try {
      const next = await action();
      if (mounted.current && next) setStatus(next);
    } catch (error) {
      if (mounted.current) setStatus(isSessionExpiredError(error) ? "session-expired" : "error");
    } finally {
      if (mounted.current) setBusy(false);
    }
  }, []);

  const enable = useCallback(() => run(subscribeDevice), [run]);
  const disable = useCallback(
    () =>
      run(async () => {
        await unsubscribeDevice();
        return "unsubscribed";
      }),
    [run],
  );
  const refresh = useCallback(() => run(detectStatus), [run]);

  const sendTest = useCallback(async () => {
    setBusy(true);
    try {
      await sendTestPush();
      if (mounted.current) setTestResult("sent");
    } catch (error) {
      if (!mounted.current) return;
      if (error instanceof ApiError && error.status === 404) setTestResult("no-subscription");
      else if (isSessionExpiredError(error)) setStatus("session-expired");
      else setTestResult("failed");
    } finally {
      if (mounted.current) setBusy(false);
    }
  }, []);

  return { status, busy, testResult, enable, disable, refresh, sendTest };
}
