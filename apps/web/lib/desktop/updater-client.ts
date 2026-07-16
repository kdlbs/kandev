import { createTauriInvokeTransport } from "./tauri-event-transport";
import { createDesktopUpdaterAdapter } from "./updater-adapter";

export const desktopUpdater = createDesktopUpdaterAdapter(createTauriInvokeTransport());
