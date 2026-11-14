import { fetchJson, type ApiRequestOptions } from "../client";
import { nativeDashboard, nativeLedger, nativeTasks } from "@/lib/control-ui/native-control";
import type {
  NativeArchive,
  NativeDashboard,
  NativeLedger,
  NativeTaskList,
} from "@/lib/types/control-native";

export class NativeControlContractError extends Error {}

export async function readNativeLedger(id: string, options?: ApiRequestOptions) {
  const value = await fetchJson<unknown>(
    `/api/v1/workspaces/${encodeURIComponent(id)}/stats/model-usage?source=ledger&range=day`,
    options,
  );
  if (!nativeLedger(value, id)) {
    // i18n-exempt: transport diagnostic; rendered copy is localized.
    throw new NativeControlContractError("Unsupported native control projection");
  }
  return value as NativeLedger;
}

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

export async function readNativeTasks(
  id: string,
  filter: string,
  archive: NativeArchive,
  limit: number,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ view: "control", filter, archive, limit: String(limit) });
  const value = await fetchJson<unknown>(
    `/api/v1/office/workspaces/${encodeURIComponent(id)}/tasks?${query}`,
    options,
  );
  if (!nativeTasks(value, id, filter, archive)) {
    // i18n-exempt: transport diagnostic; rendered copy is localized.
    throw new NativeControlContractError("Unsupported native control projection");
  }
  return value as NativeTaskList;
}
