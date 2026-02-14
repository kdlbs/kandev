'use client';

import { createElement, useCallback, useRef, useState } from 'react';
import { mergeAttributes, Node } from '@tiptap/core';
import { ReactNodeViewRenderer, NodeViewWrapper, type ReactNodeViewProps } from '@tiptap/react';
import { Suggestion, type SuggestionOptions } from '@tiptap/suggestion';
import { IconFile, IconFolder, IconAt, IconListCheck } from '@tabler/icons-react';
import { HoverCard, HoverCardTrigger, HoverCardContent } from '@kandev/ui/hover-card';
import { isDirectory } from '@/lib/utils/file-path';
import { usePanelActions } from '@/hooks/use-panel-actions';
import { useEditorContext } from './editor-context';
import { LazyFilePreview } from './context-items/lazy-file-preview';
import { LazyPlanPreview } from './context-items/lazy-plan-preview';
import { PromptPreviewFromStore } from './context-items/prompt-preview';

export type MentionKind = 'file' | 'prompt' | 'plan';

export type ContextMentionOptions = {
  /** Suggestion configs — one per trigger character (e.g. @ and /) */
  suggestions: Array<Partial<SuggestionOptions>>;
};

/**
 * Custom mention node extension that renders as an inline chip.
 * Stores kind/path as attributes so we know what was mentioned.
 * Accepts multiple suggestion plugins (@ mentions and / commands).
 */
export const ContextMention = Node.create<ContextMentionOptions>({
  name: 'contextMention',
  group: 'inline',
  inline: true,
  selectable: false,
  atom: true,

  addOptions() {
    return {
      suggestions: [],
    };
  },

  addAttributes() {
    return {
      id: { default: null },
      label: { default: null },
      kind: { default: 'file' as MentionKind },
      path: { default: null },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-context-mention]' }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'span',
      mergeAttributes(
        { 'data-context-mention': '' },
        HTMLAttributes,
      ),
      HTMLAttributes.label || HTMLAttributes.id || '',
    ];
  },

  renderText({ node }) {
    return `@${node.attrs.label ?? node.attrs.id}`;
  },

  addNodeView() {
    return ReactNodeViewRenderer(MentionChipView);
  },

  addProseMirrorPlugins() {
    return this.options.suggestions.map((suggestion) =>
      Suggestion({
        editor: this.editor,
        ...suggestion,
      }),
    );
  },
});

function MentionChipView({ node }: ReactNodeViewProps) {
  const { id, label, kind, path } = node.attrs as { id: string; label: string; kind: MentionKind; path: string };
  const { sessionId, taskId } = useEditorContext();
  const { openFile, addPlan } = usePanelActions();
  const [hoverOpen, setHoverOpen] = useState(false);
  const suppressRef = useRef(false);

  const clickable = kind === 'file' || kind === 'plan';

  const handleClick = useCallback(() => {
    suppressRef.current = true;
    setHoverOpen(false);
    setTimeout(() => { suppressRef.current = false; }, 300);
    if (kind === 'file' && path) {
      openFile(path);
    } else if (kind === 'plan') {
      addPlan();
    }
  }, [kind, path, openFile, addPlan]);

  const icon = getMentionIcon(kind, path);

  const preview = useMentionPreview(kind, path, id, sessionId, taskId);

  const chip = (
    <span
      contentEditable={false}
      className={`inline-flex items-center gap-1 bg-muted/70 rounded px-1.5 py-0.5 text-xs align-baseline ${clickable ? 'cursor-pointer hover:bg-muted' : 'cursor-default'}`}
      onClick={clickable ? handleClick : undefined}
    >
      {createElement(icon, { className: "h-3 w-3 shrink-0 text-muted-foreground" })}
      <span className="truncate max-w-[140px]">{label}</span>
    </span>
  );

  if (!preview) {
    return <NodeViewWrapper as="span" className="inline">{chip}</NodeViewWrapper>;
  }

  return (
    <NodeViewWrapper as="span" className="inline">
      <HoverCard
        open={hoverOpen}
        onOpenChange={(next) => {
          if (next && suppressRef.current) return;
          setHoverOpen(next);
        }}
        openDelay={300}
      >
        <HoverCardTrigger asChild>{chip}</HoverCardTrigger>
        <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
          {preview}
        </HoverCardContent>
      </HoverCard>
    </NodeViewWrapper>
  );
}

function useMentionPreview(
  kind: MentionKind,
  path: string,
  id: string,
  sessionId: string | null,
  taskId: string | null,
): React.ReactNode {
  if (kind === 'file') {
    return <LazyFilePreview path={path} sessionId={sessionId} />;
  }

  if (kind === 'prompt') {
    return <PromptPreviewFromStore promptId={id} />;
  }

  if (kind === 'plan') {
    return <LazyPlanPreview taskId={taskId} />;
  }

  return null;
}

function getMentionIcon(kind: MentionKind, path?: string) {
  if (kind === 'prompt') return IconAt;
  if (kind === 'plan') return IconListCheck;
  if (path && isDirectory(path)) return IconFolder;
  return IconFile;
}
