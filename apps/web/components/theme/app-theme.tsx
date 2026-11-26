"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
  useRef,
  type ReactNode,
} from "react";

import type {
  ThemeSelection as Theme,
  EffectiveTheme,
  ThemeInventory,
} from "@/lib/theme/theme-types";
import { isThemeSelection, resolveEffectiveTheme } from "@/lib/theme/theme-resolver";
import { applyThemeColors } from "@/lib/theme/theme-dom";
import { useThemeInventory } from "@/hooks/domains/plugins/use-theme-inventory";
import { t } from "@/lib/i18n";

export type ResolvedTheme = "light" | "dark";

type ThemeContextValue = {
  theme: Theme;
  savedTheme: Theme;
  setTheme: (theme: string) => void;
  previewTheme: (theme: string) => void;
  commitTheme: (theme: string) => void;
  restoreTheme: () => void;
  resolvedTheme: ResolvedTheme;
  systemTheme: ResolvedTheme;
  effectiveTheme: EffectiveTheme;
  themeInventory: ThemeInventory | null;
  themeError: string | null;
  refreshThemes: () => void;
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
  try {
    const stored = window.localStorage.getItem(storageKey);
    return isThemeSelection(stored) ? stored : fallback;
  } catch {
    return fallback;
  }
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

export function AppThemeProvider({
  children,
  attribute = "class",
  defaultTheme = "system",
  enableSystem = true,
  disableTransitionOnChange = false,
  storageKey = "theme",
}: AppThemeProviderProps) {
  const [theme, setThemeState] = useState<Theme>(() => readStoredTheme(storageKey, defaultTheme));
  const [savedTheme, setSavedTheme] = useState<Theme>(() =>
    readStoredTheme(storageKey, defaultTheme),
  );
  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(() => getSystemTheme());
  const { inventory, definitions, error, refresh } = useThemeInventory(theme);
  const bases = useRef(new Map<string, ResolvedTheme>());
  const effectiveTheme = useMemo(
    () =>
      resolveEffectiveTheme(
        theme,
        enableSystem ? systemTheme : "light",
        inventory,
        definitions,
        bases.current.get(theme),
      ),
    [theme, enableSystem, systemTheme, inventory, definitions],
  );
  const resolvedTheme = effectiveTheme.base;
  useEffect(() => {
    if (effectiveTheme.definition) bases.current.set(theme, effectiveTheme.base);
  }, [theme, effectiveTheme]);

  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!media) return;
    const listener = () => setSystemTheme(getSystemTheme());
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, []);

  useLayoutEffect(() => {
    const update = () => {
      applyTheme(attribute, resolvedTheme);
      applyThemeColors(document.documentElement, effectiveTheme);
    };
    if (disableTransitionOnChange) {
      withDisabledTransitions(update);
      return;
    }
    update();
  }, [attribute, disableTransitionOnChange, resolvedTheme, effectiveTheme]);

  const commitTheme = useCallback(
    (nextTheme: string) => {
      if (!isThemeSelection(nextTheme)) return;
      if (nextTheme.startsWith("theme:")) {
        const candidate = resolveEffectiveTheme(nextTheme, systemTheme, inventory, definitions);
        if (candidate.status !== "ready") throw new Error(t("plugins:failedToLoadPlugins"));
      }
      window.localStorage.setItem(storageKey, nextTheme);
      setSavedTheme(nextTheme);
      setThemeState(nextTheme);
    },
    [storageKey, systemTheme, inventory, definitions],
  );

  const previewTheme = useCallback((nextTheme: string) => {
    if (!isThemeSelection(nextTheme)) return;
    setThemeState(nextTheme);
  }, []);

  const restoreTheme = useCallback(() => {
    setThemeState(savedTheme);
  }, [savedTheme]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      savedTheme,
      setTheme: commitTheme,
      previewTheme,
      commitTheme,
      restoreTheme,
      resolvedTheme,
      systemTheme,
      effectiveTheme,
      themeInventory: inventory,
      themeError: error,
      refreshThemes: refresh,
    }),
    [
      commitTheme,
      previewTheme,
      resolvedTheme,
      restoreTheme,
      savedTheme,
      systemTheme,
      theme,
      effectiveTheme,
      inventory,
      error,
      refresh,
    ],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (value) return value;
  return {
    theme: "system",
    savedTheme: "system",
    setTheme: () => {},
    previewTheme: () => {},
    commitTheme: () => {},
    restoreTheme: () => {},
    resolvedTheme: getSystemTheme(),
    systemTheme: getSystemTheme(),
    effectiveTheme: {
      reference: "system",
      base: getSystemTheme(),
      digest: getSystemTheme(),
      status: "ready",
    },
    themeInventory: null,
    themeError: null,
    refreshThemes: () => {},
  };
}
