import type { SessionModelsPayload } from "@/lib/types/backend";

export type SessionModelConfigOption = SessionModelsPayload["config_options"][number];

export type PersistedRuntimeConfig = {
  model?: string;
  configOptions?: Record<string, string>;
  baseline?: Record<string, string>;
};
