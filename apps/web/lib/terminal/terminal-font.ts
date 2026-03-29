export type FontCategory = "icons" | "ligatures" | "system";

export type TerminalFontPreset = {
  value: string;
  label: string;
  category: FontCategory;
};

export const TERMINAL_FONT_PRESETS: TerminalFontPreset[] = [
  // Nerd Fonts (icon support)
  { value: "JetBrainsMono Nerd Font", label: "JetBrains Mono Nerd Font", category: "icons" },
  { value: "FiraCode Nerd Font", label: "Fira Code Nerd Font", category: "icons" },
  { value: "MesloLGS NF", label: "MesloLGS NF", category: "icons" },
  { value: "Hack Nerd Font", label: "Hack Nerd Font", category: "icons" },
  { value: "CaskaydiaCove Nerd Font", label: "Cascadia Code Nerd Font", category: "icons" },
  // Programming fonts (ligatures)
  { value: "JetBrains Mono", label: "JetBrains Mono", category: "ligatures" },
  { value: "Fira Code", label: "Fira Code", category: "ligatures" },
  { value: "Cascadia Code", label: "Cascadia Code", category: "ligatures" },
  { value: "Source Code Pro", label: "Source Code Pro", category: "ligatures" },
  // System monospace
  { value: "Menlo", label: "Menlo", category: "system" },
  { value: "Monaco", label: "Monaco", category: "system" },
  { value: "Consolas", label: "Consolas", category: "system" },
  { value: "SF Mono", label: "SF Mono", category: "system" },
];

const DEFAULT_FONT_FAMILY = 'Menlo, Monaco, "Courier New", monospace';

export function buildTerminalFontFamily(selected: string | null): string {
  if (!selected) return DEFAULT_FONT_FAMILY;
  return `"${selected}", ${DEFAULT_FONT_FAMILY}`;
}
