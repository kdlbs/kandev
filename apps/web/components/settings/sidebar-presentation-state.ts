import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
export type SidebarPresentation = { fast: boolean; style: "simple" | "compact" };
export function presentationPatch(
  draft: SidebarPresentation,
  saved: SidebarPresentation,
): UserSettingsUpdatePayload {
  return {
    ...(draft.fast !== saved.fast ? { sidebar_fast_actions_enabled: draft.fast } : {}),
    ...(draft.style !== saved.style ? { sidebar_new_task_style: draft.style } : {}),
  };
}
export function rebasePresentation(
  draft: SidebarPresentation,
  baseline: SidebarPresentation,
  saved: SidebarPresentation,
): SidebarPresentation {
  return {
    fast: draft.fast === baseline.fast ? saved.fast : draft.fast,
    style: draft.style === baseline.style ? saved.style : draft.style,
  };
}
