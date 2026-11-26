import { tags, type Tag } from "@lezer/highlight";
import { vscodeDarkInit, vscodeLightInit } from "@uiw/codemirror-theme-vscode";
import type { EffectiveTheme } from "./theme-types";

const syntaxTags: Record<string, Tag | Tag[]> = {
  comment: tags.comment,
  keyword: tags.keyword,
  string: tags.string,
  number: tags.number,
  boolean: tags.bool,
  type: tags.typeName,
  function: tags.function(tags.variableName),
  variable: tags.variableName,
  parameter: tags.variableName,
  property: tags.propertyName,
  constant: tags.constant(tags.name),
  operator: tags.operator,
  punctuation: tags.punctuation,
  tag: tags.tagName,
  attribute: tags.attributeName,
};
export function codeMirrorTheme(theme: EffectiveTheme) {
  const d = theme.definition;
  const colors = d?.editor ?? {};
  const ui = d?.ui ?? {};
  const styles = Object.entries(d?.syntax ?? {}).map(([key, style]) => ({
    tag: syntaxTags[key],
    color: style.foreground,
    backgroundColor: style.background,
    fontStyle: style.fontStyle?.includes("italic") ? "italic" : undefined,
    fontWeight: style.fontStyle?.includes("bold") ? "bold" : undefined,
    textDecoration: [
      style.fontStyle?.includes("underline") ? "underline" : "",
      style.fontStyle?.includes("strikethrough") ? "line-through" : "",
    ]
      .filter(Boolean)
      .join(" "),
  }));
  const settings = Object.fromEntries(
    Object.entries({
      background: colors.background ?? ui.background,
      foreground: colors.foreground ?? ui.foreground,
      caret: colors.cursor,
      selection: colors.selection,
      selectionMatch: colors["inactive-selection"],
      lineHighlight: colors["line-highlight"],
      gutterBackground: colors.gutter ?? colors.background ?? ui.background,
      gutterForeground: colors["line-number"],
      gutterActiveForeground: colors["active-line-number"],
    }).filter(([, value]) => value),
  );
  return (theme.base === "dark" ? vscodeDarkInit : vscodeLightInit)({ settings, styles });
}
