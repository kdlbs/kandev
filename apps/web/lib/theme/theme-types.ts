export type ThemeBase = "light" | "dark";
export type ThemeSelection = ThemeBase | "system" | `theme:${string}/${string}`;
export type TokenStyle = { foreground?: string; background?: string; fontStyle?: string };
export type ThemeDefinition = {
  schema_version: 1;
  base: ThemeBase;
  ui?: Record<string, string>;
  editor?: Record<string, string>;
  syntax?: Record<string, TokenStyle>;
  terminal?: Record<string, string>;
  textmate?: { scope: string | string[]; settings: TokenStyle }[];
  semantic?: Record<string, TokenStyle>;
};
export type ThemeDescriptor = {
  package_id: string;
  package_name: string;
  key: string;
  label: string;
  base: ThemeBase;
  version: string;
  digest: string;
};
export type ThemeInventory = { boot_id: string; revision: number; themes: ThemeDescriptor[] };
export type EffectiveTheme = {
  reference: ThemeSelection;
  base: ThemeBase;
  digest: string;
  definition?: ThemeDefinition;
  status: "ready" | "loading" | "unavailable" | "error";
};
