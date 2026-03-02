import { useAppStore } from "@/components/state-provider";

/** Returns true when the user has configured a default utility agent. */
export function useIsUtilityConfigured(): boolean {
  return useAppStore((s) => !!s.userSettings.defaultUtilityAgentId);
}
