import { fetchJson, type ApiRequestOptions } from "../client";
import {
  nativeDashboard,
  nativeHistory,
  nativeLedger,
  nativeTasks,
} from "@/lib/control-ui/native-control";
import type {
  NativeArchive,
  NativeHistory,
  NativeDashboard,
  NativeTaskList,
  NativeLedger,
} from "@/lib/types/control-native";
import type { OverviewTaskFilter } from "@/lib/types/control-overview";

export class NativeControlContractError extends Error {}
function checked<T>(value: unknown, check: (value: unknown) => value is T): T {
  if (!check(value)) {
    // i18n-exempt: transport diagnostic; rendered copy is localized.
    throw new NativeControlContractError("Unsupported native control projection");
  }
  return value;
}
const root = (id: string) => `/api/v1/office/workspaces/${encodeURIComponent(id)}`;
export async function readNativeHistory(id: string, options?: ApiRequestOptions) {
  return checked(
    await fetchJson<unknown>(`${root(id)}/activity?view=control&window_hours=24`, options),
    (v): v is NativeHistory => nativeHistory(v, id),
  );
}
export async function readNativeDashboard(id: string, options?: ApiRequestOptions) {
  return checked(
    await fetchJson<unknown>(`${root(id)}/dashboard?view=control`, options),
    (v): v is NativeDashboard => nativeDashboard(v, id),
  );
}
export async function readNativeTasks(
  id: string,
  filter: OverviewTaskFilter,
  archive: NativeArchive,
  limit: number,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ view: "control", filter, archive, limit: String(limit) });
  return checked(
    await fetchJson<unknown>(`${root(id)}/tasks?${query}`, options),
    (v): v is NativeTaskList => nativeTasks(v, id, filter, archive),
  );
}
export async function readNativeLedger(id: string, options?: ApiRequestOptions) {
  return checked(
    await fetchJson<unknown>(
      `/api/v1/workspaces/${encodeURIComponent(id)}/stats/model-usage?source=ledger&range=day`,
      options,
    ),
    (v): v is NativeLedger => nativeLedger(v, id),
  );
}
