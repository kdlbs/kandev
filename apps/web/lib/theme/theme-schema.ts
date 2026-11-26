import { z } from "zod";
import schema from "../../../backend/internal/plugins/themes/schema/v1.json";
import type { ThemeDefinition } from "./theme-types";

export const themeRoleKeys = {
  ui: Object.keys(schema.properties.ui.properties),
  editor: Object.keys(schema.properties.editor.properties),
  terminal: Object.keys(schema.properties.terminal.properties),
  syntax: Object.keys(schema.properties.syntax.properties),
};
const color = z.string().regex(/^#(?:[\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})$/i);
const style = z.strictObject({
  foreground: color.optional(),
  background: color.optional(),
  fontStyle: z
    .string()
    .regex(new RegExp(schema.properties.syntax.properties.comment.properties.fontStyle.pattern))
    .optional(),
});
const roleColors = (keys: string[]) => z.partialRecord(z.enum(keys), color);
const scope = z.string().min(1).max(256);
const definition = z.strictObject({
  schema_version: z.literal(1),
  base: z.enum(["light", "dark"]),
  ui: roleColors(themeRoleKeys.ui).optional(),
  editor: roleColors(themeRoleKeys.editor).optional(),
  terminal: roleColors(themeRoleKeys.terminal).optional(),
  syntax: z.partialRecord(z.enum(themeRoleKeys.syntax), style).optional(),
  textmate: z
    .array(
      z.strictObject({ scope: z.union([scope, z.array(scope).min(1).max(4096)]), settings: style }),
    )
    .max(1024)
    .optional(),
  semantic: z
    .record(
      z.string().regex(new RegExp(Object.keys(schema.properties.semantic.patternProperties)[0])),
      style,
    )
    .optional(),
});

export function parseThemeDefinition(value: unknown): ThemeDefinition | null {
  if (JSON.stringify(value)?.length > 256 * 1024) return null;
  const parsed = definition.safeParse(value);
  if (!parsed.success) return null;
  const d = parsed.data;
  const ui = d.ui ?? {};
  const backgrounds = [
    ui.background,
    ui.card,
    ui.popover,
    ui.sidebar,
    d.editor?.background,
    d.terminal?.background,
  ];
  if (
    backgrounds.some(
      (c) =>
        c &&
        ((c.length === 9 && !c.toLowerCase().endsWith("ff")) ||
          (c.length === 5 && !c.toLowerCase().endsWith("f"))),
    )
  )
    return null;
  const scopes =
    d.textmate?.reduce(
      (sum, rule) => sum + (Array.isArray(rule.scope) ? rule.scope.length : 1),
      0,
    ) ?? 0;
  return scopes <= 4096 && Object.keys(d.semantic ?? {}).length <= 256
    ? (d as ThemeDefinition)
    : null;
}
