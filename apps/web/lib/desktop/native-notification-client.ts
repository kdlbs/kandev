import { createTauriInvokeTransport } from "./tauri-event-transport";

export type NativeNotificationRequest = {
  eventId: string;
  title: string;
  body: string;
  /** Absent for account/app-scoped events (e.g. an available update) that have no task. */
  taskId?: string;
  sessionId?: string | null;
};

export type NativeNotificationResult =
  | "shown"
  | "duplicate"
  | "permission-denied"
  | "display-failed";

export type NativeNotificationPermission =
  | "granted"
  | "denied"
  | "prompt"
  | "prompt-with-rationale";

const transport = createTauriInvokeTransport();

export const nativeNotifications = {
  isAvailable: transport.isAvailable,
  permission: {
    get(): Promise<NativeNotificationPermission> {
      return transport.invoke(
        "get_native_notification_permission",
      ) as Promise<NativeNotificationPermission>;
    },
    request(): Promise<NativeNotificationPermission> {
      return transport.invoke(
        "request_native_notification_permission",
      ) as Promise<NativeNotificationPermission>;
    },
  },
  show(request: NativeNotificationRequest): Promise<NativeNotificationResult> {
    if (!transport.isAvailable()) return Promise.resolve("duplicate");
    return transport.invoke("show_native_notification", {
      request,
    }) as Promise<NativeNotificationResult>;
  },
};
