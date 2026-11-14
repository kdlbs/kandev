/**
 * The slice of the service worker global scope the worker uses. Declared here
 * because the app's tsconfig loads the DOM lib, which conflicts with
 * `lib.webworker`; these shapes match the Service Worker spec.
 */
export interface SwWindowClient {
  url: string;
  focused: boolean;
  visibilityState: string;
  focus(): Promise<unknown>;
  postMessage(message: unknown): void;
}

export interface SwNotification {
  close(): void;
  data: unknown;
}

interface ExtendableEventLike extends Event {
  waitUntil(promise: Promise<unknown>): void;
}

export interface SwFetchEvent extends ExtendableEventLike {
  request: Request;
  respondWith(response: Promise<Response> | Response): void;
}

export interface SwPushEvent extends ExtendableEventLike {
  data: { text(): string } | null;
}

export interface SwNotificationEvent extends ExtendableEventLike {
  notification: SwNotification;
}

export interface ServiceWorkerScope {
  location: Location;
  caches: CacheStorage;
  clients: {
    claim(): Promise<void>;
    matchAll(options: { type: "window"; includeUncontrolled: boolean }): Promise<SwWindowClient[]>;
    openWindow(url: string): Promise<unknown>;
  };
  registration: {
    showNotification(title: string, options?: NotificationOptions): Promise<void>;
  };
  skipWaiting(): Promise<void>;
  addEventListener(
    type: "install" | "activate",
    listener: (event: ExtendableEventLike) => void,
  ): void;
  addEventListener(type: "fetch", listener: (event: SwFetchEvent) => void): void;
  addEventListener(type: "push", listener: (event: SwPushEvent) => void): void;
  addEventListener(type: "notificationclick", listener: (event: SwNotificationEvent) => void): void;
}
