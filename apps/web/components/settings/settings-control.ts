import { cn } from "@/lib/utils";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { SETTINGS_TYPOGRAPHY } from "./settings-typography";

/** Editable/select settings controls use the shared responsive size contract. */
export function settingsControlClassName(className?: string) {
  return cn(controlSizingClassName("standard"), SETTINGS_TYPOGRAPHY.control, className);
}

/** Credential and secret fields share the technical value treatment. */
export function settingsCredentialClassName(className?: string) {
  return settingsControlClassName(cn("font-mono", className));
}

/** Settings actions use the same responsive size contract as editable controls. */
export function settingsActionClassName(className?: string) {
  return cn(controlSizingClassName("standard"), SETTINGS_TYPOGRAPHY.mobileAction, className);
}
