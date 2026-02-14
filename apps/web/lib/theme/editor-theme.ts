import type { editor } from 'monaco-editor';

// Semantic token color rules matching VS Code Dark+ / Light+ themes.
// These color function names, types, variables, etc. when an LSP server
// provides semantic tokens (textDocument/semanticTokens/full).
const SEMANTIC_RULES_DARK: editor.ITokenThemeRule[] = [
  // Types — teal
  { token: 'type', foreground: '4EC9B0' },
  { token: 'class', foreground: '4EC9B0' },
  { token: 'struct', foreground: '4EC9B0' },
  { token: 'interface', foreground: '4EC9B0' },
  { token: 'enum', foreground: '4EC9B0' },
  { token: 'typeParameter', foreground: '4EC9B0' },
  { token: 'namespace', foreground: '4EC9B0' },
  // Functions — yellow
  { token: 'function', foreground: 'DCDCAA' },
  { token: 'method', foreground: 'DCDCAA' },
  { token: 'macro', foreground: 'DCDCAA' },
  // Variables — light blue
  { token: 'variable', foreground: '9CDCFE' },
  { token: 'parameter', foreground: '9CDCFE' },
  { token: 'property', foreground: '9CDCFE' },
  { token: 'enumMember', foreground: '4FC1FF' },
  // Decorators / annotations
  { token: 'decorator', foreground: 'DCDCAA' },
];

const SEMANTIC_RULES_LIGHT: editor.ITokenThemeRule[] = [
  // Types — dark teal
  { token: 'type', foreground: '267f99' },
  { token: 'class', foreground: '267f99' },
  { token: 'struct', foreground: '267f99' },
  { token: 'interface', foreground: '267f99' },
  { token: 'enum', foreground: '267f99' },
  { token: 'typeParameter', foreground: '267f99' },
  { token: 'namespace', foreground: '267f99' },
  // Functions — brown
  { token: 'function', foreground: '795E26' },
  { token: 'method', foreground: '795E26' },
  { token: 'macro', foreground: '795E26' },
  // Variables — dark blue
  { token: 'variable', foreground: '001080' },
  { token: 'parameter', foreground: '001080' },
  { token: 'property', foreground: '001080' },
  { token: 'enumMember', foreground: '0070C1' },
  // Decorators
  { token: 'decorator', foreground: '795E26' },
];

export const KANDEV_MONACO_DARK: editor.IStandaloneThemeData = {
  base: 'vs-dark',
  inherit: true,
  rules: [...SEMANTIC_RULES_DARK],
  colors: {
    'editor.background': '#141414',
    'editor.foreground': '#d4d4d4',
    'editorGutter.background': '#141414',
    'editor.lineHighlightBackground': '#1c1c1c',
    'editorLineNumber.foreground': '#555555',
    'editorLineNumber.activeForeground': '#888888',
    'editor.selectionBackground': '#264f78',
    'editor.inactiveSelectionBackground': '#3a3d41',
    'editorCursor.foreground': '#d4d4d4',
    'editorWidget.background': '#222222',
    'editorWidget.border': '#2a2a2a',
    'editorSuggestWidget.background': '#222222',
    'editorSuggestWidget.border': '#2a2a2a',
    'editorSuggestWidget.selectedBackground': '#264f78',
    'minimap.background': '#141414',
    'scrollbar.shadow': '#00000000',
    'scrollbarSlider.background': '#64646480',
    'scrollbarSlider.hoverBackground': '#82828299',
    'scrollbarSlider.activeBackground': '#828282bb',
    'diffEditor.insertedTextBackground': '#10b98126',
    'diffEditor.removedTextBackground': '#f43f5e26',
    'diffEditor.insertedLineBackground': '#10b98115',
    'diffEditor.removedLineBackground': '#f43f5e15',
  },
};

export const KANDEV_MONACO_LIGHT: editor.IStandaloneThemeData = {
  base: 'vs',
  inherit: true,
  rules: [...SEMANTIC_RULES_LIGHT],
  colors: {
    'editor.background': '#ffffff',
    'editor.foreground': '#1e1e1e',
    'editorGutter.background': '#ffffff',
    'editor.lineHighlightBackground': '#f5f5f5',
    'editorLineNumber.foreground': '#c0c0c0',
    'editorLineNumber.activeForeground': '#555555',
    'diffEditor.insertedTextBackground': '#10b98126',
    'diffEditor.removedTextBackground': '#f43f5e26',
    'diffEditor.insertedLineBackground': '#10b98115',
    'diffEditor.removedLineBackground': '#f43f5e15',
  },
};

export const PIERRE_DIFFS_THEME = {
  dark: 'github-dark-high-contrast' as const,
  light: 'github-light' as const,
};

export const EDITOR_FONT_FAMILY =
  '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
export const EDITOR_FONT_SIZE = 12;
export const EDITOR_LINE_HEIGHT = 18;
