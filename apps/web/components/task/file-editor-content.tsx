'use client';

import { useCallback, useEffect, useMemo } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { EditorView } from '@codemirror/view';
import type { Extension } from '@codemirror/state';
import { getCodeMirrorExtensionFromPath } from '@/lib/languages';
import { Button } from '@kandev/ui/button';
import { IconDeviceFloppy, IconLoader2 } from '@tabler/icons-react';
import { formatDiffStats } from '@/lib/utils/file-diff';

type FileEditorContentProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  isSaving: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
};

export function FileEditorContent({
  path,
  content,
  originalContent,
  isDirty,
  isSaving,
  onChange,
  onSave,
}: FileEditorContentProps) {
  const langExt = getCodeMirrorExtensionFromPath(path);
  const extensions: Extension[] = [
    EditorView.lineWrapping,
    EditorView.editable.of(true),
  ];
  if (langExt) {
    extensions.push(langExt);
  }

  // Calculate diff stats when dirty
  const diffStats = useMemo(() => {
    if (!isDirty) return null;
    
    // Simple line-based diff for stats
    const originalLines = originalContent.split('\n');
    const currentLines = content.split('\n');
    
    let additions = 0;
    let deletions = 0;
    
    // Simple comparison - count different lines
    const maxLen = Math.max(originalLines.length, currentLines.length);
    for (let i = 0; i < maxLen; i++) {
      const origLine = originalLines[i];
      const currLine = currentLines[i];
      
      if (origLine === undefined && currLine !== undefined) {
        additions++;
      } else if (origLine !== undefined && currLine === undefined) {
        deletions++;
      } else if (origLine !== currLine) {
        // Line changed - count as both addition and deletion
        additions++;
        deletions++;
      }
    }
    
    return { additions, deletions };
  }, [isDirty, content, originalContent]);

  // Handle Cmd/Ctrl+S keyboard shortcut
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        if (isDirty && !isSaving) {
          onSave();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isDirty, isSaving, onSave]);

  const handleChange = useCallback(
    (value: string) => {
      onChange(value);
    },
    [onChange]
  );

  return (
    <div className="flex h-full flex-col">
      {/* Editor header with save button */}
      <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span className="font-mono">{path}</span>
          {isDirty && diffStats && (
            <span className="text-xs text-yellow-500">
              {formatDiffStats(diffStats.additions, diffStats.deletions)}
            </span>
          )}
        </div>
        <Button
          size="sm"
          variant="default"
          onClick={onSave}
          disabled={!isDirty || isSaving}
          className="cursor-pointer gap-2"
        >
          {isSaving ? (
            <>
              <IconLoader2 className="h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : (
            <>
              <IconDeviceFloppy className="h-4 w-4" />
              Save
              <span className="text-xs text-muted-foreground">
                ({navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}+S)
              </span>
            </>
          )}
        </Button>
      </div>

      {/* CodeMirror editor */}
      <div className="flex-1 overflow-hidden">
        <CodeMirror
          value={content}
          height="100%"
          theme={vscodeDark}
          extensions={extensions}
          onChange={handleChange}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
          }}
          className="h-full overflow-auto text-sm"
        />
      </div>
    </div>
  );
}

