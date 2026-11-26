import { themeRoleKeys } from "./theme-schema";
import type { EffectiveTheme } from "./theme-types";

export function applyThemeColors(root: HTMLElement, theme: EffectiveTheme): void {
  for (const group of ["ui", "editor", "terminal"] as const) {
    for (const key of themeRoleKeys[group]) {
      const property = group === "ui" ? `--${key}` : `--theme-${group}-${key}`;
      root.style.removeProperty(property);
      const color = theme.definition?.[group]?.[key];
      if (color) root.style.setProperty(property, key.startsWith("git-") ? hexToRgb(color) : color);
    }
  }
  root.style.colorScheme = theme.base;
  root.dataset.codeTheme = theme.digest;
}

function hexToRgb(color: string): string {
  const hex =
    color.length <= 5
      ? color
          .slice(1, 4)
          .split("")
          .map((v) => v + v)
          .join("")
      : color.slice(1, 7);
  return [0, 2, 4].map((i) => Number.parseInt(hex.slice(i, i + 2), 16)).join(" ");
}
