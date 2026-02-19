import { themeAbyss, type DockviewTheme } from "dockview-react";

export const themeKandev: DockviewTheme = {
  ...themeAbyss,
  className: `${themeAbyss.className} dockview-theme-kandev`,
  gap: 0,
};
