import type {
  UserShellKind,
  UserShellState,
  UserShellPTYStatus,
} from "@/lib/state/slices/session-runtime/types";

export type TerminalType = "dev-server" | "shell" | "script";

/**
 * Terminal tab descriptor consumed by the right-panel UI. Ordinary
 * terminals carry the new `kind="ordinary"` discriminator plus seq +
 * customName + state + ptyStatus; non-ordinary tabs (dev-server, scripts,
 * the fixed bottom-panel) leave those undefined.
 */
export type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
  closable: boolean;
  kind?: UserShellKind;
  seq?: number;
  customName?: string | null;
  state?: UserShellState;
  ptyStatus?: UserShellPTYStatus;
};
