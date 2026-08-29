export {};

declare global {
  interface KandevWindowControlsOverlay {
    readonly visible: boolean;
    getTitlebarAreaRect(): DOMRectReadOnly;
    addEventListener(type: "geometrychange", listener: EventListener): void;
    removeEventListener(type: "geometrychange", listener: EventListener): void;
  }

  interface Navigator {
    readonly windowControlsOverlay?: KandevWindowControlsOverlay;
  }

  interface Window {
    // Port injection for dev mode (browser on web port, API on backend port)
    __KANDEV_API_PORT?: string;
    // Debug mode flag (injected by the Go shell or derived from boot payload runtime config)
    __KANDEV_DEBUG?: boolean;
  }
}
