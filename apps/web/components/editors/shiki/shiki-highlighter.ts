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

export async function highlightCode(code: string, lang: string, isDark: boolean): Promise<string> {
  const highlighter = await getHighlighter();
  const hasLang = await ensureLanguage(highlighter, lang);
  return highlighter.codeToHtml(code, {
    lang: hasLang ? lang : "text",
    theme: isDark ? DARK_THEME : LIGHT_THEME,
  });
}
