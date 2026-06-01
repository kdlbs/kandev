import { useUserSettings } from "@/hooks/domains/settings/use-user-settings";

/** Returns true when the user has configured a default utility agent. */
export function useIsUtilityConfigured(): boolean {
  return !!useUserSettings().data?.defaultUtilityAgentId;
}
