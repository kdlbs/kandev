import type { editor } from 'monaco-editor';
import { DARK, LIGHT, DIFF_COLORS, FONT } from './colors';

// Semantic token color rules matching VS Code Dark+ / Light+ themes.
const SEMANTIC_RULES_DARK: editor.ITokenThemeRule[] = [
  { token: 'type', foreground: '4EC9B0' },
  { token: 'class', foreground: '4EC9B0' },
  { token: 'struct', foreground: '4EC9B0' },
  { token: 'interface', foreground: '4EC9B0' },
  { token: 'enum', foreground: '4EC9B0' },
  { token: 'typeParameter', foreground: '4EC9B0' },
  { token: 'namespace', foreground: '4EC9B0' },
  { token: 'function', foreground: 'DCDCAA' },
  { token: 'method', foreground: 'DCDCAA' },
  { token: 'macro', foreground: 'DCDCAA' },
  { token: 'variable', foreground: '9CDCFE' },
  { token: 'parameter', foreground: '9CDCFE' },
  { token: 'property', foreground: '9CDCFE' },
  { token: 'enumMember', foreground: '4FC1FF' },
  { token: 'decorator', foreground: 'DCDCAA' },
];

const SEMANTIC_RULES_LIGHT: editor.ITokenThemeRule[] = [
  { token: 'type', foreground: '267f99' },
  { token: 'class', foreground: '267f99' },
  { token: 'struct', foreground: '267f99' },
  { token: 'interface', foreground: '267f99' },
  { token: 'enum', foreground: '267f99' },
  { token: 'typeParameter', foreground: '267f99' },
  { token: 'namespace', foreground: '267f99' },
  { token: 'function', foreground: '795E26' },
  { token: 'method', foreground: '795E26' },
  { token: 'macro', foreground: '795E26' },
  { token: 'variable', foreground: '001080' },
  { token: 'parameter', foreground: '001080' },
  { token: 'property', foreground: '001080' },
  { token: 'enumMember', foreground: '0070C1' },
  { token: 'decorator', foreground: '795E26' },
];

export const KANDEV_MONACO_DARK: editor.IStandaloneThemeData = {
  base: 'vs-dark',
  inherit: true,
  rules: [...SEMANTIC_RULES_DARK],
  colors: {
    'editor.background': DARK.bg,
    'editor.foreground': DARK.fg,
    'editorGutter.background': DARK.bg,
    'editor.lineHighlightBackground': DARK.lineHighlight,
    'editorLineNumber.foreground': DARK.lineNumber,
    'editorLineNumber.activeForeground': DARK.lineNumberActive,
    'editor.selectionBackground': DARK.selection,
    'editor.inactiveSelectionBackground': DARK.selectionInactive,
    'editorCursor.foreground': DARK.cursor,
    'editorWidget.background': DARK.popover,
    'editorWidget.border': DARK.border,
    'editorSuggestWidget.background': DARK.popover,
    'editorSuggestWidget.border': DARK.border,
    'editorSuggestWidget.selectedBackground': DARK.selection,
    'minimap.background': DARK.bg,
    'scrollbar.shadow': DARK.scrollbarShadow,
    'scrollbarSlider.background': DARK.scrollbarThumb,
    'scrollbarSlider.hoverBackground': DARK.scrollbarThumbHover,
    'scrollbarSlider.activeBackground': DARK.scrollbarThumbActive,
    'diffEditor.insertedTextBackground': DIFF_COLORS.additionTextBg,
    'diffEditor.removedTextBackground': DIFF_COLORS.deletionTextBg,
    'diffEditor.insertedLineBackground': DIFF_COLORS.additionLineBg,
    'diffEditor.removedLineBackground': DIFF_COLORS.deletionLineBg,
  },
};

export const KANDEV_MONACO_LIGHT: editor.IStandaloneThemeData = {
  base: 'vs',
  inherit: true,
  rules: [...SEMANTIC_RULES_LIGHT],
  colors: {
    'editor.background': LIGHT.bg,
    'editor.foreground': LIGHT.fg,
    'editorGutter.background': LIGHT.bg,
    'editor.lineHighlightBackground': LIGHT.lineHighlight,
    'editorLineNumber.foreground': LIGHT.lineNumber,
    'editorLineNumber.activeForeground': LIGHT.lineNumberActive,
    'diffEditor.insertedTextBackground': DIFF_COLORS.additionTextBg,
    'diffEditor.removedTextBackground': DIFF_COLORS.deletionTextBg,
    'diffEditor.insertedLineBackground': DIFF_COLORS.additionLineBg,
    'diffEditor.removedLineBackground': DIFF_COLORS.deletionLineBg,
  },
};

export const EDITOR_FONT_FAMILY = FONT.mono;
export const EDITOR_FONT_SIZE = FONT.size;
export const EDITOR_LINE_HEIGHT = FONT.lineHeight;
