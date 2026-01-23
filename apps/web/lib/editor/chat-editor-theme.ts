import { EditorView } from '@codemirror/view';

export const chatEditorTheme = EditorView.theme({
  '&': {
    color: 'var(--foreground)',
    fontSize: '0.8125rem', // 13px
    backgroundColor: 'transparent',
  },
  '.cm-editor': {
    borderRadius: 'inherit',
    background: 'transparent',
    outline: 'none',
    boxShadow: 'none',
    overflow: 'hidden',
  },
  '.cm-editor.cm-focused': {
    outline: 'none',
    boxShadow: 'none',
  },
  '.cm-scroller': {
    fontFamily: 'var(--font-sans)',
    borderRadius: 'inherit',
    background: 'transparent',
    backgroundColor: 'transparent',
    overflow: 'auto',
  },
  '.cm-content': {
    padding: '0.5rem',
    fontFamily: 'var(--font-sans)',
    color: 'var(--foreground)',
    background: 'transparent',
  },
  '.cm-line': {
    color: 'var(--foreground)',
  },
  '.cm-placeholder': {
    color: 'var(--muted-foreground)',
    opacity: '0.8',
  },
  '.cm-tooltip-autocomplete, .cm-tooltip-autocomplete li, .cm-completionDetail': {
    fontSize: '0.8125rem', // 13px
  },
  '.cm-cursor': {
    borderLeftColor: 'var(--foreground)',
  },
  '.cm-selectionBackground': {
    backgroundColor: '#6366f1 !important',
  },
  '.cm-selectionMatch': {
    backgroundColor: '#818cf8 !important',
  },
  '.cm-selectionLayer .cm-selectionBackground': {
    backgroundColor: '#6366f1 !important',
  },
  '&.cm-focused .cm-selectionBackground': {
    backgroundColor: '#6366f1 !important',
  },
  '.cm-line ::selection': {
    backgroundColor: '#6366f1 !important',
  },
  '.cm-content ::selection': {
    backgroundColor: '#6366f1 !important',
  },
});
