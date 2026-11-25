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

export const SETTINGS_SWITCH_CLASS_NAME =
  "data-[state=checked]:bg-transparent data-[state=unchecked]:bg-transparent dark:data-[state=unchecked]:bg-transparent data-[size=default]:h-7 data-[size=default]:w-9 max-md:data-[size=default]:h-11 max-md:data-[size=default]:w-11 [@media(pointer:coarse)]:data-[size=default]:h-11 [@media(pointer:coarse)]:data-[size=default]:w-11 cursor-pointer shrink-0 p-1 max-md:p-2 [@media(pointer:coarse)]:p-2 before:absolute before:left-1 max-md:before:left-2 [@media(pointer:coarse)]:before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-[state=checked]:before:bg-primary dark:data-[state=unchecked]:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10";
