import { fetchJson, type ApiRequestOptions } from "../client";
import { nativeDashboard } from "@/lib/control-ui/native-control";
import type { NativeDashboard } from "@/lib/types/control-native";

export class NativeControlContractError extends Error {}

export async function readNativeDashboard(id: string, options?: ApiRequestOptions) {
  const value = await fetchJson<unknown>(
    `/api/v1/office/workspaces/${encodeURIComponent(id)}/dashboard?view=control`,
    options,
  );
  if (!nativeDashboard(value, id)) {
    // i18n-exempt: transport diagnostic; rendered copy is localized.
    throw new NativeControlContractError("Unsupported native control projection");
  }
  return value as NativeDashboard;
}
