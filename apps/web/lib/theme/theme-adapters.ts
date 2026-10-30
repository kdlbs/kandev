import type { editor } from "monaco-editor";
import type { ThemeRegistration } from "shiki";
import type { EffectiveTheme } from "./theme-types";
import { KANDEV_MONACO_DARK, KANDEV_MONACO_LIGHT } from "./editor-theme";

export const editorColorNames: Record<string, string> = {
  background: "editor.background",
  foreground: "editor.foreground",
  cursor: "editorCursor.foreground",
  selection: "editor.selectionBackground",
  "inactive-selection": "editor.inactiveSelectionBackground",
  "line-highlight": "editor.lineHighlightBackground",
  "line-number": "editorLineNumber.foreground",
  "active-line-number": "editorLineNumber.activeForeground",
  gutter: "editorGutter.background",
  "widget-background": "editorWidget.background",
  "widget-foreground": "editorWidget.foreground",
  "widget-border": "editorWidget.border",
  error: "editorError.foreground",
  warning: "editorWarning.foreground",
  info: "editorInfo.foreground",
  "diff-added-line": "diffEditor.insertedLineBackground",
  "diff-removed-line": "diffEditor.removedLineBackground",
  "diff-added-text": "diffEditor.insertedTextBackground",
  "diff-removed-text": "diffEditor.removedTextBackground",
};
const portableScopes: Record<string, string[]> = {
  comment: ["comment"],
  keyword: ["keyword", "storage"],
  string: ["string"],
  number: ["constant.numeric"],
  boolean: ["constant.language.boolean"],
  type: ["entity.name.type", "support.type"],
  function: ["entity.name.function", "support.function"],
  variable: ["variable"],
  parameter: ["variable.parameter"],
  property: ["variable.other.property"],
  constant: ["constant"],
  operator: ["keyword.operator"],
  punctuation: ["punctuation"],
  tag: ["entity.name.tag"],
  attribute: ["entity.other.attribute-name"],
};
export function monacoThemeData(theme: EffectiveTheme): editor.IStandaloneThemeData {
  const base = theme.base === "dark" ? KANDEV_MONACO_DARK : KANDEV_MONACO_LIGHT;
  const colors = { ...base.colors };
  const d = theme.definition ?? { schema_version: 1, base: theme.base };
  if (d.ui?.background) colors["editor.background"] = d.ui.background;
  if (d.ui?.foreground) colors["editor.foreground"] = d.ui.foreground;
  for (const [key, color] of Object.entries(d.editor ?? {}))
    if (editorColorNames[key]) colors[editorColorNames[key]] = color;
  const rules = [...base.rules];
  for (const [token, style] of Object.entries({ ...d.syntax, ...d.semantic })) {
    rules.push({
      token,
      foreground: style.foreground?.slice(1),
      background: style.background?.slice(1),
      fontStyle: style.fontStyle,
    });
  }
  return { ...base, colors, rules };
}
export function textmateThemeData(
  theme: EffectiveTheme,
  fallback?: ThemeRegistration,
): ThemeRegistration {
  const data = monacoThemeData(theme);
  const d = theme.definition ?? { schema_version: 1, base: theme.base };
  const portable = Object.entries(d.syntax ?? {}).map(([role, settings]) => ({
    scope: portableScopes[role] ?? [role],
    settings,
  }));
  const inherited = (fallback?.tokenColors ?? fallback?.settings ?? [])
    .filter((rule) => rule.scope)
    .map((rule) => {
      const scopes = Array.isArray(rule.scope) ? rule.scope : String(rule.scope).split(",");
      const settings = { ...rule.settings };
      for (const [role, style] of Object.entries(d.syntax ?? {})) {
        if (
          scopes.some((scope) =>
            portableScopes[role]?.some(
              (prefix) => scope.trim() === prefix || scope.trim().startsWith(`${prefix}.`),
            ),
          )
        )
          Object.assign(settings, style);
      }
      return { ...rule, settings };
    });
  const colors = { ...fallback?.colors, ...data.colors };
  return {
    name: `kandev-${theme.digest}`,
    type: theme.base,
    colors,
    tokenColors: [
      {
        settings: {
          foreground: colors["editor.foreground"],
          background: colors["editor.background"],
        },
      },
      ...inherited,
      ...portable,
      ...(d.textmate ?? []),
    ],
  };
}
