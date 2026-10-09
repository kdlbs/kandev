export const LSP_DEFAULT_CONFIGS: Record<string, Record<string, unknown>> = {
  go: { "ui.semanticTokens": true },
};

export const DISABLED_LSP_STATUS = { state: "disabled" } as const;
export const LSP_IDLE_TIMEOUT = 2 * 60 * 1_000;
export const LSP_RECONNECT_DELAYS_MS = [250, 500, 1_000, 2_000, 4_000] as const;
export const LSP_RELEASE_ACK_TIMEOUT_MS = 3_500;
