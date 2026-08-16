import { cn } from "@/lib/utils";
import { SETTINGS_TYPOGRAPHY } from "./settings-typography";

/** Editable/select settings controls: touch-sized on phones, compact on desktop. */
export function settingsControlClassName(className?: string) {
  return cn("min-h-11 md:min-h-7", SETTINGS_TYPOGRAPHY.control, className);
}

/** Credential and secret fields share the technical value treatment. */
export function settingsCredentialClassName(className?: string) {
  return settingsControlClassName(cn("font-mono", className));
}

/** Settings actions use the same touch contract as editable controls. */
export function settingsActionClassName(className?: string) {
  return cn(SETTINGS_TYPOGRAPHY.mobileAction, className);
}
