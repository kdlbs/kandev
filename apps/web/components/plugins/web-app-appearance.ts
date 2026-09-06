export const WEB_APP_APPEARANCE_TYPE = "kandev.web_app.appearance";
export const WEB_APP_APPEARANCE_VERSION = 1;

export const WEB_APP_APPEARANCE_TOKEN_KEYS = [
  "background",
  "foreground",
  "card",
  "cardForeground",
  "muted",
  "mutedForeground",
  "border",
  "primary",
  "primaryForeground",
  "accent",
  "accentForeground",
  "destructive",
  "destructiveForeground",
  "ring",
] as const;

export type WebAppAppearanceToken = (typeof WEB_APP_APPEARANCE_TOKEN_KEYS)[number];
export type WebAppAppearanceMode = "light" | "dark";

export type WebAppAppearanceMessage = {
  type: typeof WEB_APP_APPEARANCE_TYPE;
  version: typeof WEB_APP_APPEARANCE_VERSION;
  mode: WebAppAppearanceMode;
  tokens: Record<WebAppAppearanceToken, string>;
};

const CSS_VARIABLES: Record<WebAppAppearanceToken, string> = {
  background: "--background",
  foreground: "--foreground",
  card: "--card",
  cardForeground: "--card-foreground",
  muted: "--muted",
  mutedForeground: "--muted-foreground",
  border: "--border",
  primary: "--primary",
  primaryForeground: "--primary-foreground",
  accent: "--accent",
  accentForeground: "--accent-foreground",
  destructive: "--destructive",
  destructiveForeground: "--destructive-foreground",
  ring: "--ring",
};

const LIGHT_FOREGROUND = "oklch(0.145 0 0)";
const DARK_FOREGROUND = "oklch(0.985 0 0)";

const FALLBACK_TOKENS: Record<WebAppAppearanceMode, Record<WebAppAppearanceToken, string>> = {
  light: {
    background: "oklch(1 0 0)",
    foreground: LIGHT_FOREGROUND,
    card: "oklch(1 0 0)",
    cardForeground: LIGHT_FOREGROUND,
    muted: "oklch(0.97 0 0)",
    mutedForeground: "oklch(0.556 0 0)",
    border: "oklch(0.922 0 0)",
    primary: "oklch(0.51 0.23 277)",
    primaryForeground: "oklch(0.96 0.02 272)",
    accent: "oklch(0.68 0.18 276)",
    accentForeground: "oklch(0.98 0 0)",
    destructive: "oklch(0.58 0.22 27)",
    destructiveForeground: "oklch(0.96 0.02 27)",
    ring: "oklch(0.708 0 0)",
  },
  dark: {
    background: LIGHT_FOREGROUND,
    foreground: DARK_FOREGROUND,
    card: "oklch(0.205 0 0)",
    cardForeground: DARK_FOREGROUND,
    muted: "oklch(0.269 0 0)",
    mutedForeground: "oklch(0.708 0 0)",
    border: "oklch(1 0 0 / 10%)",
    primary: "oklch(0.65 0.2 277)",
    primaryForeground: LIGHT_FOREGROUND,
    accent: "oklch(0.45 0.16 276)",
    accentForeground: DARK_FOREGROUND,
    destructive: "oklch(0.704 0.191 22.216)",
    destructiveForeground: DARK_FOREGROUND,
    ring: "oklch(0.556 0 0)",
  },
};

const SAFE_COLOR_VALUE = /^[#a-z0-9(),.%/\s.+-]+$/i;
const SAFE_COLOR_FUNCTION = /^(?:oklch|oklab|rgb|rgba|hsl|hsla|hwb|lab|lch|color)\([^{};]+\)$/i;
const SAFE_COLOR_KEYWORDS = new Set(["transparent", "currentcolor"]);

export function isBoundedCssColor(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const trimmed = value.trim();
  if (trimmed.length === 0 || trimmed.length > 128) return false;
  if (!SAFE_COLOR_VALUE.test(trimmed)) return false;
  if (/^(?:inherit|initial|unset|revert(?:-layer)?|none|url|var|expression)$/i.test(trimmed)) {
    return false;
  }
  if (/^#[0-9a-f]{3,8}$/i.test(trimmed) || SAFE_COLOR_FUNCTION.test(trimmed)) return true;
  if (SAFE_COLOR_KEYWORDS.has(trimmed.toLowerCase())) return true;
  if (typeof CSS !== "undefined" && typeof CSS.supports === "function") {
    return CSS.supports("color", trimmed);
  }
  return false;
}

function resolvedMode(doc: Document, mode?: WebAppAppearanceMode): WebAppAppearanceMode {
  if (mode) return mode;
  return doc.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function resolveWebAppAppearance(
  doc: Document = document,
  mode?: WebAppAppearanceMode,
): WebAppAppearanceMessage {
  const resolved = resolvedMode(doc, mode);
  const fallback = FALLBACK_TOKENS[resolved];
  const tokens = {} as Record<WebAppAppearanceToken, string>;
  let styles: CSSStyleDeclaration | null = null;
  try {
    styles = doc.defaultView?.getComputedStyle(doc.documentElement) ?? null;
  } catch {
    styles = null;
  }

  for (const key of WEB_APP_APPEARANCE_TOKEN_KEYS) {
    const value = styles?.getPropertyValue(CSS_VARIABLES[key]).trim() ?? "";
    tokens[key] = isBoundedCssColor(value) ? value : fallback[key];
  }

  return {
    type: WEB_APP_APPEARANCE_TYPE,
    version: WEB_APP_APPEARANCE_VERSION,
    mode: resolved,
    tokens,
  };
}
