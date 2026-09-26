import type { ResolvedTheme } from "./app-theme";

export type ThemeToggleLabelKey =
  | "common:commandSwitchToDarkMode"
  | "common:commandSwitchToLightMode";

export function getThemeToggleTarget(resolvedTheme: ResolvedTheme): ResolvedTheme {
  return resolvedTheme === "dark" ? "light" : "dark";
}

export function getThemeToggleLabelKey(resolvedTheme: ResolvedTheme): ThemeToggleLabelKey {
  return resolvedTheme === "dark"
    ? "common:commandSwitchToLightMode"
    : "common:commandSwitchToDarkMode";
}
