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
const requestedEventIds = new Set<string>();

export const nativeNotifications = {
  isAvailable: transport.isAvailable,
  show(request: NativeNotificationRequest): Promise<NativeNotificationResult> {
    if (!transport.isAvailable()) return Promise.resolve("duplicate");
    if (requestedEventIds.has(request.eventId)) return Promise.resolve("duplicate");
    requestedEventIds.add(request.eventId);
    return transport.invoke("show_native_notification", {
      request,
    }) as Promise<NativeNotificationResult>;
  },
};
