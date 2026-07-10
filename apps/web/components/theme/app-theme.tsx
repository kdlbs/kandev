"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type Theme = "light" | "dark" | "system";
type ResolvedTheme = "light" | "dark";

type ThemeContextValue = {
  theme: Theme;
  setTheme: (theme: string) => void;
  resolvedTheme: ResolvedTheme;
  systemTheme: ResolvedTheme;
};

type AppThemeProviderProps = {
  children: ReactNode;
  attribute?: "class" | `data-${string}`;
  defaultTheme?: Theme;
  enableSystem?: boolean;
  disableTransitionOnChange?: boolean;
  storageKey?: string;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function readStoredTheme(storageKey: string, fallback: Theme): Theme {
  if (typeof window === "undefined") return fallback;
  const stored = window.localStorage.getItem(storageKey);
  return stored === "light" || stored === "dark" || stored === "system" ? stored : fallback;
}

function getSystemTheme(): ResolvedTheme {
  if (typeof window === "undefined") return "light";
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function withDisabledTransitions(update: () => void) {
  const style = document.createElement("style");
  style.appendChild(document.createTextNode("*{transition:none!important}"));
  document.head.appendChild(style);
  update();
  window.getComputedStyle(document.body);
  window.setTimeout(() => style.remove(), 0);
}

function applyTheme(attribute: AppThemeProviderProps["attribute"], resolvedTheme: ResolvedTheme) {
  const root = document.documentElement;
  if (attribute === "class") {
    root.classList.remove("light", "dark");
    root.classList.add(resolvedTheme);
    return;
  }
  root.setAttribute(attribute ?? "data-theme", resolvedTheme);
}

function resolveTheme(theme: Theme, enableSystem: boolean, systemTheme: ResolvedTheme) {
  if (theme === "system" && enableSystem) return systemTheme;
  return theme === "dark" ? "dark" : "light";
}

export function AppThemeProvider({
  children,
  attribute = "class",
  defaultTheme = "system",
  enableSystem = true,
  disableTransitionOnChange = false,
  storageKey = "theme",
}: AppThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(() => readStoredTheme(storageKey, defaultTheme));
  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(() => getSystemTheme());
  const resolvedTheme = resolveTheme(theme, enableSystem, systemTheme);

  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!media) return;
    const listener = () => setSystemTheme(getSystemTheme());
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, []);

  useEffect(() => {
    const update = () => applyTheme(attribute, resolvedTheme);
    if (disableTransitionOnChange) {
      withDisabledTransitions(update);
      return;
    }
    update();
  }, [attribute, disableTransitionOnChange, resolvedTheme]);

  const setTheme = useCallback(
    (nextTheme: string) => {
      if (nextTheme !== "light" && nextTheme !== "dark" && nextTheme !== "system") return;
      window.localStorage.setItem(storageKey, nextTheme);
      setThemeState(nextTheme);
    },
    [storageKey],
  );

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, setTheme, resolvedTheme, systemTheme }),
    [resolvedTheme, setTheme, systemTheme, theme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (value) return value;
  return {
    theme: "system",
    setTheme: () => {},
    resolvedTheme: getSystemTheme(),
    systemTheme: getSystemTheme(),
  };
}
