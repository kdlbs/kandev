import type { EffectiveTheme } from "@/lib/theme/theme-types";
import { textmateThemeData } from "@/lib/theme/theme-adapters";
import { createHighlighter, type Highlighter } from "shiki";
let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLanguages = new Set<string>();

const DARK_THEME = "dark-plus";
const LIGHT_THEME = "light-plus";

function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: [DARK_THEME, LIGHT_THEME],
      langs: [],
    });
  }
  return highlighterPromise;
}

async function ensureLanguage(highlighter: Highlighter, lang: string): Promise<boolean> {
  if (!lang || loadedLanguages.has(lang)) return loadedLanguages.has(lang);
  try {
    await highlighter.loadLanguage(lang as Parameters<Highlighter["loadLanguage"]>[0]);
    loadedLanguages.add(lang);
    return true;
  } catch {
    return false;
  }
}

export async function highlightCode(
  code: string,
  lang: string,
  isDark: boolean,
  custom?: EffectiveTheme,
): Promise<string> {
  const highlighter = await getHighlighter();
  const hasLang = await ensureLanguage(highlighter, lang);
  let themeName = isDark ? DARK_THEME : LIGHT_THEME;
  if (custom?.definition) {
    const theme = textmateThemeData(custom, highlighter.getTheme(themeName));
    themeName = theme.name!;
    if (!highlighter.getLoadedThemes().includes(themeName)) await highlighter.loadTheme(theme);
  }
  return highlighter.codeToHtml(code, {
    lang: hasLang ? lang : "text",
    theme: themeName,
  });
}
