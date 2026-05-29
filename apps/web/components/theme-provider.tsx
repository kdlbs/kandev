"use client";

import { ThemeProvider as NextThemesProvider } from "next-themes";
import { ReactNode } from "react";

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem={true}
      // Suppress per-element color transitions during a theme flip so every
      // surface (buttons, panels, backgrounds) switches in the same instant
      // instead of each animating at its own `transition-colors` duration.
      disableTransitionOnChange
    >
      {children}
    </NextThemesProvider>
  );
}
