export type FontCategory = "icons" | "ligatures" | "system";

export type TerminalFontPreset = {
  value: string;
  label: string;
  category: FontCategory;
};

export const TERMINAL_FONT_PRESETS: TerminalFontPreset[] = [
  // Nerd Fonts (icon/glyph support)
  {
    value: '"JetBrainsMono Nerd Font", "JetBrains Mono", Menlo, Consolas, monospace',
    label: "JetBrains Mono Nerd Font",
    category: "icons",
  },
  {
    value: '"FiraCode Nerd Font", "Fira Code", Menlo, Consolas, monospace',
    label: "Fira Code Nerd Font",
    category: "icons",
  },
  {
    value: '"MesloLGS NF", "MesloLGS Nerd Font", Menlo, Consolas, monospace',
    label: "MesloLGS Nerd Font",
    category: "icons",
  },
  {
    value: '"Hack Nerd Font", Hack, Menlo, Consolas, monospace',
    label: "Hack Nerd Font",
    category: "icons",
  },
  {
    value: '"CaskaydiaCove Nerd Font", "Cascadia Code", Menlo, Consolas, monospace',
    label: "Cascadia Code Nerd Font",
    category: "icons",
  },
  // Programming fonts (ligatures)
  {
    value: '"JetBrains Mono", "Fira Code", Menlo, Consolas, monospace',
    label: "JetBrains Mono",
    category: "ligatures",
  },
  {
    value: '"Fira Code", "JetBrains Mono", Menlo, Consolas, monospace',
    label: "Fira Code",
    category: "ligatures",
  },
  {
    value: '"Cascadia Code", "Cascadia Mono", Menlo, Consolas, monospace',
    label: "Cascadia Code",
    category: "ligatures",
  },
  {
    value: '"Source Code Pro", "DejaVu Sans Mono", Menlo, Consolas, monospace',
    label: "Source Code Pro",
    category: "ligatures",
  },
  // System monospace (cross-platform fallback chains)
  {
    value: 'Menlo, "DejaVu Sans Mono", Consolas, monospace',
    label: "System Default",
    category: "system",
  },
  {
    value: '"SF Mono", Menlo, "DejaVu Sans Mono", Consolas, monospace',
    label: "SF Mono",
    category: "system",
  },
  {
    value: '"Ubuntu Mono", "DejaVu Sans Mono", Menlo, Consolas, monospace',
    label: "Ubuntu Mono",
    category: "system",
  },
];

const DEFAULT_FONT_FAMILY = 'Menlo, Monaco, "Courier New", monospace';

export const DEFAULT_FONT_SIZE = 13;

export function buildTerminalFontFamily(selected: string | null): string {
  if (!selected) return DEFAULT_FONT_FAMILY;
  return selected;
}
