import { createTauriInvokeTransport } from "./tauri-event-transport";

export type NativeNotificationRequest = {
  eventId: string;
  title: string;
  body: string;
  taskId: string;
  sessionId?: string | null;
};

type NativeNotificationResult = "shown" | "duplicate" | "permission-denied";

const transport = createTauriInvokeTransport();

export const nativeNotifications = {
  isAvailable: transport.isAvailable,
  show(request: NativeNotificationRequest): Promise<NativeNotificationResult> {
    if (!transport.isAvailable()) return Promise.resolve("duplicate");
    return transport.invoke("show_native_notification", {
      request,
    }) as Promise<NativeNotificationResult>;
  },
};
