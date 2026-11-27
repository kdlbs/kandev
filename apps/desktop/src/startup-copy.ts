import en from "./locales/en.json";
import ja from "./locales/ja.json";
import ptPt from "./locales/pt-pt.json";
import zhCn from "./locales/zh-cn.json";
import zhHk from "./locales/zh-hk.json";
import zhTw from "./locales/zh-tw.json";

const dictionaries = {
  en,
  "pt-pt": ptPt,
  "zh-cn": zhCn,
  "zh-hk": zhHk,
  "zh-tw": zhTw,
  ja,
} as const;

export type StartupLocale = keyof typeof dictionaries;
export type StartupCopyKey = keyof typeof en;

export const startupLocale = resolveStartupLocale(navigator.languages ?? [navigator.language]);

export function getStartupText(
  key: StartupCopyKey,
  values: Record<string, string | number> = {},
): string {
  let value: string = dictionaries[startupLocale][key];
  for (const [name, replacement] of Object.entries(values)) {
    value = value.replaceAll(`{{${name}}}`, String(replacement));
  }
  return value;
}

function resolveStartupLocale(languages: readonly string[]): StartupLocale {
  for (const language of languages) {
    const normalized = language.toLowerCase().replaceAll("_", "-");
    if (normalized.startsWith("pt")) return "pt-pt";
    if (normalized.startsWith("ja")) return "ja";
    if (normalized.startsWith("zh")) {
      if (/^zh-(?:hant-)?(?:hk|mo)(-|$)/.test(normalized)) return "zh-hk";
      if (/^zh-(?:hant-)?tw(-|$)/.test(normalized)) return "zh-tw";
      if (/^zh-hant(?:-|$)/.test(normalized)) return "zh-tw";
      return "zh-cn";
    }
  }
  return "en";
}
