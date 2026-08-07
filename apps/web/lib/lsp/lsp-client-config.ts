export const LSP_DEFAULT_CONFIGS: Record<string, Record<string, unknown>> = {
  go: { "ui.semanticTokens": true },
};

export const DISABLED_LSP_STATUS = { state: "disabled" } as const;
export const LSP_IDLE_TIMEOUT = 2 * 60 * 1_000;
