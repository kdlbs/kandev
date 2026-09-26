import type { MarketplaceEntry } from "@/lib/types/plugins";

/** The row's view of its marketplace-update status. */
export type PluginRowUpdateState = {
  latest?: MarketplaceEntry;
  hasUpdate: boolean;
  checked: boolean;
  sourcesDegraded?: boolean;
  busy: boolean;
  error?: string;
};
