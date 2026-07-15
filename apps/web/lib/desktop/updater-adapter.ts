import { DESKTOP_NATIVE_COMMANDS, type DesktopUpdateState } from "./protocol";

export type DesktopInvokeTransport = {
  isAvailable: () => boolean;
  invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
};

export type DesktopUpdaterAdapter = {
  isAvailable: () => boolean;
  getState: () => Promise<DesktopUpdateState>;
  checkForUpdates: () => Promise<DesktopUpdateState>;
  installUpdate: () => Promise<DesktopUpdateState>;
};

export function createDesktopUpdaterAdapter(
  transport: DesktopInvokeTransport,
): DesktopUpdaterAdapter {
  const invoke = (command: keyof typeof DESKTOP_NATIVE_COMMANDS) => {
    if (!transport.isAvailable()) {
      return Promise.reject(new Error("The desktop updater is unavailable."));
    }
    return transport.invoke(DESKTOP_NATIVE_COMMANDS[command]) as Promise<DesktopUpdateState>;
  };
  return {
    isAvailable: transport.isAvailable,
    getState: () => invoke("get-update-state"),
    checkForUpdates: () => invoke("check-for-updates"),
    installUpdate: () => invoke("install-update"),
  };
}
