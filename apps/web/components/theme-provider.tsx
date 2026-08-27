"use client";

import { AppThemeProvider, useTheme } from "@/components/theme/app-theme";
import { ReactNode, useEffect } from "react";

const TITLEBAR_THEME_COLORS = {
  light: "#ffffff",
  dark: "#181818",
} as const;

function DesktopPwaThemeColor() {
  const { resolvedTheme } = useTheme();

  useEffect(() => {
    if (typeof navigator === "undefined" || !navigator.windowControlsOverlay) return;

    const themeColors = Array.from(
      document.head.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]'),
    );
    const activeThemeColor = themeColors[0] ?? document.createElement("meta");
    if (!activeThemeColor.isConnected) {
      activeThemeColor.name = "theme-color";
      document.head.appendChild(activeThemeColor);
    }
    activeThemeColor.content = TITLEBAR_THEME_COLORS[resolvedTheme];
    activeThemeColor.removeAttribute("media");
    themeColors.slice(1).forEach((themeColor) => themeColor.remove());
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
