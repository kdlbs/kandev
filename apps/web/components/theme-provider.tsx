"use client";

import { AppThemeProvider } from "@/components/theme/app-theme";
import { ReactNode } from "react";

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
      {children}
    </AppThemeProvider>
  );
}
