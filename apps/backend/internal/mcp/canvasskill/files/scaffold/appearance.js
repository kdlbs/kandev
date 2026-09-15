(() => {
  "use strict";

  const type = "kandev.web_app.appearance";
  const version = 1;
  const keys = [
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
  ];
  const variables = {
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
  const fallbacks = {
    light: {
      background: "oklch(1 0 0)",
      foreground: "oklch(0.145 0 0)",
      card: "oklch(1 0 0)",
      cardForeground: "oklch(0.145 0 0)",
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
      background: "oklch(0.145 0 0)",
      foreground: "oklch(0.985 0 0)",
      card: "oklch(0.205 0 0)",
      cardForeground: "oklch(0.985 0 0)",
      muted: "oklch(0.269 0 0)",
      mutedForeground: "oklch(0.708 0 0)",
      border: "oklch(1 0 0 / 10%)",
      primary: "oklch(0.65 0.2 277)",
      primaryForeground: "oklch(0.145 0 0)",
      accent: "oklch(0.45 0.16 276)",
      accentForeground: "oklch(0.985 0 0)",
      destructive: "oklch(0.704 0.191 22.216)",
      destructiveForeground: "oklch(0.985 0 0)",
      ring: "oklch(0.556 0 0)",
    },
  };
  const colorPattern = /^[#a-z0-9(),.%/\s.+-]+$/i;
  const colorFunctionPattern = /^(?:oklch|oklab|rgb|rgba|hsl|hsla|hwb|lab|lch|color)\([^{};]+\)$/i;

  function isColor(value) {
    if (typeof value !== "string") return false;
    const color = value.trim();
    if (color.length === 0 || color.length > 128 || !colorPattern.test(color)) return false;
    return (
      /^#[0-9a-f]{3,8}$/i.test(color) ||
      colorFunctionPattern.test(color) ||
      color === "transparent" ||
      color === "currentcolor"
    );
  }

  function isAppearanceMessage(value) {
    if (!value || value.type !== type || value.version !== version) return false;
    if (value.mode !== "light" && value.mode !== "dark") return false;
    if (!value.tokens || typeof value.tokens !== "object") return false;
    const tokenKeys = Object.keys(value.tokens);
    return (
      tokenKeys.length === keys.length &&
      keys.every((key) => tokenKeys.includes(key) && isColor(value.tokens[key]))
    );
  }

  function applyAppearance(value) {
    keys.forEach((key) => {
      document.documentElement.style.setProperty(variables[key], value.tokens[key]);
    });
    document.documentElement.style.colorScheme = value.mode;
    document.documentElement.dataset.kandevAppearance = value.mode;
  }

  const initialMode = document.documentElement.dataset.kandevAppearance === "dark" ? "dark" : "light";
  applyAppearance({ tokens: fallbacks[initialMode], mode: initialMode });
  window.addEventListener("message", (event) => {
    if (event.source === window.parent && isAppearanceMessage(event.data)) {
      applyAppearance(event.data);
    }
  });
})();
