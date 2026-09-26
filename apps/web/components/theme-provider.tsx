"use client";

import { AppThemeProvider, useTheme } from "@/components/theme/app-theme";
import { ReactNode, useEffect } from "react";

const TITLEBAR_THEME_COLORS = {
  light: "#ffffff",
  dark: "#181818",
} as const;
const OVERLAY_THEME_COLOR_SELECTOR = 'meta[data-kandev-window-controls-theme-color="true"]';

function DesktopPwaThemeColor() {
  const { resolvedTheme } = useTheme();

  useEffect(() => {
    if (typeof navigator === "undefined") return;
    const overlay = navigator.windowControlsOverlay;
    if (!overlay) return;

    const syncThemeColor = () => {
      const activeThemeColor = document.head.querySelector<HTMLMetaElement>(
        OVERLAY_THEME_COLOR_SELECTOR,
      );
      if (!overlay.visible) {
        activeThemeColor?.remove();
        return;
      }

      const themeColor = activeThemeColor ?? document.createElement("meta");
      themeColor.name = "theme-color";
      themeColor.content = TITLEBAR_THEME_COLORS[resolvedTheme];
      themeColor.dataset.kandevWindowControlsThemeColor = "true";
      if (!activeThemeColor) document.head.prepend(themeColor);
    };

    syncThemeColor();
    overlay.addEventListener("geometrychange", syncThemeColor);
    return () => {
      overlay.removeEventListener("geometrychange", syncThemeColor);
      document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR)?.remove();
    };
  }, [resolvedTheme]);

  return null;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <AppThemeProvider
      attribute="class"
      defaultTheme="system"
      enableSystem={true}
      // Suppress per-element color transitions during a theme flip so every
      // surface (buttons, panels, backgrounds) switches in the same instant
      // instead of each animating at its own `transition-colors` duration.
      disableTransitionOnChange
    >
      <DesktopPwaThemeColor />
      {children}
    </AppThemeProvider>
  );
}
