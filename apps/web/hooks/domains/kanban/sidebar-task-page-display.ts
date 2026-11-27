import type { SidebarTaskPageResponse } from "@/lib/types/http";
import { sidebarContentKey } from "./use-sidebar-page-context";

export function sidebarResponseState(
  enabled: boolean,
  loader: {
    pendingPage: number | null;
    error: string | null;
    response: SidebarTaskPageResponse | null;
    responseViewKey: string;
  },
  cached: SidebarTaskPageResponse | null,
  viewKey: string,
) {
  const current = loader.responseViewKey === viewKey ? loader.response : cached;
  const isDisclosureTransition =
    enabled &&
    !current &&
    loader.response !== null &&
    sidebarContentKey(loader.responseViewKey) === sidebarContentKey(viewKey);
  const viewResponse = current ?? (isDisclosureTransition ? loader.response : null);
  return {
    pendingPage: enabled ? loader.pendingPage : null,
    error: enabled ? loader.error : null,
    viewResponse: enabled ? viewResponse : null,
    isDisclosureTransition,
  };
}

export function sidebarLoadingState(
  enabled: boolean,
  response: SidebarTaskPageResponse | null,
  pending: number | null,
  disclosure: { collapsedGroups: string[]; transitioning: boolean },
) {
  const collapsedHeadings =
    response !== null &&
    response.entries.length > 0 &&
    response.entries.every(
      (row) => row.kind === "group" && disclosure.collapsedGroups.includes(row.group_key ?? ""),
    );
  const emptyProvisional =
    !disclosure.transitioning &&
    !collapsedHeadings &&
    response?.provisional === true &&
    !response.entries.some((row) => row.kind === "task");
  return {
    isLoading: enabled && (emptyProvisional || (pending !== null && response === null)),
    isRefreshing:
      enabled && response !== null && (pending !== null || response.provisional === true),
  };
}
