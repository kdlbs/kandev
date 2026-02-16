'use client';

import {
  forwardRef,
  useRef,
  useImperativeHandle,
  useCallback,
  useState,
  useEffect,
  useMemo,
} from 'react';
import { useEditor, EditorContent, NodeViewContent, NodeViewWrapper, ReactNodeViewRenderer, type NodeViewProps } from '@tiptap/react';
import Document from '@tiptap/extension-document';
import Paragraph from '@tiptap/extension-paragraph';
import Text from '@tiptap/extension-text';
import HardBreak from '@tiptap/extension-hard-break';
import History from '@tiptap/extension-history';
import Placeholder from '@tiptap/extension-placeholder';
import Code from '@tiptap/extension-code';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';
import { Extension } from '@tiptap/core';
import { cn } from '@/lib/utils';
import { getChatDraftContent, setChatDraftContent } from '@/lib/local-storage';
import { useCustomPrompts } from '@/hooks/domains/settings/use-custom-prompts';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import { EditorContextProvider } from './editor-context';
import { MentionMenu } from './mention-menu';
import { SlashCommandMenu } from './slash-command-menu';
import { ContextMention } from './tiptap-mention-extension';
import {
  createMentionSuggestion,
  createSlashSuggestion,
  type MenuState,
  type MentionSuggestionCallbacks,
  type SlashSuggestionCallbacks,
} from './tiptap-suggestion';
import type { MentionItem } from '@/hooks/use-inline-mention';
import type { SlashCommand } from '@/hooks/use-inline-slash';
import type { ContextFile } from '@/lib/state/context-files-store';

// ── Lowlight (syntax highlighting) ───────────────────────────────────

const lowlightInstance = createLowlight(common);

const CODE_LANGUAGES = [
  { value: '', label: 'Plain' },
  { value: 'javascript', label: 'JavaScript' },
  { value: 'typescript', label: 'TypeScript' },
  { value: 'python', label: 'Python' },
  { value: 'go', label: 'Go' },
  { value: 'rust', label: 'Rust' },
  { value: 'java', label: 'Java' },
  { value: 'cpp', label: 'C++' },
  { value: 'c', label: 'C' },
  { value: 'css', label: 'CSS' },
  { value: 'html', label: 'HTML' },
  { value: 'json', label: 'JSON' },
  { value: 'yaml', label: 'YAML' },
  { value: 'markdown', label: 'Markdown' },
  { value: 'bash', label: 'Bash' },
  { value: 'sql', label: 'SQL' },
  { value: 'xml', label: 'XML' },
];

function CodeBlockView({ node, updateAttributes }: NodeViewProps) {
  const language = (node.attrs.language as string) || '';

  return (
    <NodeViewWrapper as="pre">
      <select
        contentEditable={false}
        className="code-block-language"
        value={language}
        onChange={(e) => updateAttributes({ language: e.target.value })}
      >
        {CODE_LANGUAGES.map((lang) => (
          <option key={lang.value} value={lang.value}>
            {lang.label}
          </option>
        ))}
      </select>
      {/* @ts-expect-error -- NodeViewContent 'as' prop accepts any HTML tag but types only allow 'div' */}
      <NodeViewContent as="code" className={language ? `language-${language} hljs` : ''} />
    </NodeViewWrapper>
  );
}

// ── Handle ──────────────────────────────────────────────────────────

export type TipTapInputHandle = {
  focus: () => void;
  blur: () => void;
  getSelectionStart: () => number;
  getValue: () => string;
  setValue: (value: string) => void;
  clear: () => void;
  getTextareaElement: () => HTMLElement | null;
  insertText: (text: string, from: number, to: number) => void;
  getMentions: () => ContextFile[];
};

// ── Props ───────────────────────────────────────────────────────────

type TipTapInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  planModeEnabled?: boolean;
  submitKey?: 'enter' | 'cmd_enter';
  onFocus?: () => void;
  onBlur?: () => void;
  // TipTap-specific
  sessionId: string | null;
  taskId?: string | null;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled?: boolean;
  onAgentCommand?: (commandName: string) => void;
  onImagePaste?: (files: File[]) => void;
};

// ── Filter items ────────────────────────────────────────────────────
function filterItems(items: MentionItem[], query: string): MentionItem[] {
  if (!query) return items;
  const lq = query.toLowerCase();
  return items
    .map((item) => {
      const label = item.label.toLowerCase();
      let score = 0;
      if (label.startsWith(lq)) score = 100;
      else if (label.split(/[\s\-_/]/).some((w) => w.startsWith(lq))) score = 50;
      else if (label.includes(lq)) score = 25;
      return { item, score };
    })
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score)
    .map(({ item }) => item);
}

// ── Component ───────────────────────────────────────────────────────

export const TipTapInput = forwardRef<TipTapInputHandle, TipTapInputProps>(
  function TipTapInput(
    {
      value,
      onChange,
      onSubmit,
      placeholder = '',
      disabled = false,
      className,
      planModeEnabled = false,
      submitKey = 'cmd_enter',
      onFocus,
      onBlur,
      sessionId,
      taskId,
      onAgentCommand,
      onImagePaste,
    },
    ref,
  ) {
    // ── Refs for stable callbacks ─────────────────────────────────

    const onSubmitRef = useRef(onSubmit);
    onSubmitRef.current = onSubmit;
    const submitKeyRef = useRef(submitKey);
    submitKeyRef.current = submitKey;
    const disabledRef = useRef(disabled);
    disabledRef.current = disabled;
    const onChangeRef = useRef(onChange);
    onChangeRef.current = onChange;
    const onImagePasteRef = useRef(onImagePaste);
    onImagePasteRef.current = onImagePaste;
    const sessionIdRef = useRef(sessionId);
    sessionIdRef.current = sessionId;

    // ── Menu state ────────────────────────────────────────────────

    const [mentionMenu, setMentionMenu] = useState<MenuState<MentionItem>>({
      isOpen: false, items: [], query: '', clientRect: null, command: null,
    });
    const [slashMenu, setSlashMenu] = useState<MenuState<SlashCommand>>({
      isOpen: false, items: [], query: '', clientRect: null, command: null,
    });
    const [mentionSelectedIndex, setMentionSelectedIndex] = useState(0);
    const [slashSelectedIndex, setSlashSelectedIndex] = useState(0);

    // Reset selected index when items change
    useEffect(() => { setMentionSelectedIndex(0); }, [mentionMenu.items]);
    useEffect(() => { setSlashSelectedIndex(0); }, [slashMenu.items]);

    // ── Keyboard handler refs (set by suggestion lifecycle) ───────

    const mentionKeyDownRef = useRef<((event: KeyboardEvent) => boolean) | null>(null);
    const slashKeyDownRef = useRef<((event: KeyboardEvent) => boolean) | null>(null);

    const mentionSelectedIndexRef = useRef(mentionSelectedIndex);
    mentionSelectedIndexRef.current = mentionSelectedIndex;

    const slashSelectedIndexRef = useRef(slashSelectedIndex);
    slashSelectedIndexRef.current = slashSelectedIndex;

    // Wire up keyboard navigation for menus
    mentionKeyDownRef.current = useCallback((event: KeyboardEvent) => {
      if (!mentionMenu.isOpen) return false;
      if (event.key === 'ArrowDown') {
        setMentionSelectedIndex((i) => Math.min(i + 1, mentionMenu.items.length - 1));
        return true;
      }
      if (event.key === 'ArrowUp') {
        setMentionSelectedIndex((i) => Math.max(i - 1, 0));
        return true;
      }
      if (event.key === 'Enter' || event.key === 'Tab') {
        if (mentionMenu.items.length > 0 && mentionMenu.command) {
          const idx = mentionSelectedIndexRef.current;
          const item = mentionMenu.items[idx];
          if (item) mentionMenu.command(item);
          return true;
        }
      }
      return false;
    }, [mentionMenu]);

    slashKeyDownRef.current = useCallback((event: KeyboardEvent) => {
      if (!slashMenu.isOpen) return false;
      if (event.key === 'ArrowDown') {
        setSlashSelectedIndex((i) => Math.min(i + 1, slashMenu.items.length - 1));
        return true;
      }
      if (event.key === 'ArrowUp') {
        setSlashSelectedIndex((i) => Math.max(i - 1, 0));
        return true;
      }
      if (event.key === 'Enter' || event.key === 'Tab') {
        if (slashMenu.items.length > 0 && slashMenu.command) {
          const idx = slashSelectedIndexRef.current;
          const cmd = slashMenu.items[idx];
          if (cmd) slashMenu.command(cmd);
          return true;
        }
      }
      return false;
    }, [slashMenu]);

    // ── Data sources ──────────────────────────────────────────────

    const { prompts } = useCustomPrompts();

    const agentCommands = useAppStore((state) =>
      sessionId ? state.availableCommands.bySessionId[sessionId] : undefined
    );

    const slashCommands = useMemo((): SlashCommand[] => {
      if (!agentCommands || agentCommands.length === 0) return [];
      return agentCommands
        .filter((cmd) => {
          const desc = cmd.description || '';
          return !desc.includes('(bundled)');
        })
        .map((cmd) => ({
          id: `agent-${cmd.name}`,
          label: `/${cmd.name}`,
          description: cmd.description || `Run /${cmd.name} command`,
          action: 'agent' as const,
          agentCommandName: cmd.name,
        }));
    }, [agentCommands]);

    // ── Mention item search ───────────────────────────────────────

    const promptsRef = useRef(prompts);
    promptsRef.current = prompts;
    const lastFileSearchRef = useRef<{ query: string; results: string[] }>({ query: '', results: [] });

    const getMentionItems = useCallback(async (query: string): Promise<MentionItem[]> => {
      const allItems: MentionItem[] = [];

      // Plan item
      const planItem: MentionItem = {
        id: '__plan__',
        kind: 'plan',
        label: 'Plan',
        description: 'Include the plan as context',
        onSelect: () => {}, // unused in TipTap flow
      };
      allItems.push(planItem);

      // Prompt items
      for (const p of promptsRef.current) {
        allItems.push({
          id: p.id,
          kind: 'prompt',
          label: p.name,
          description: p.content.length > 100 ? p.content.slice(0, 100) + '...' : p.content,
          onSelect: () => {},
        });
      }

      // File search
      const sid = sessionIdRef.current;
      if (sid) {
        try {
          const client = getWebSocketClient();
          if (client) {
            const cacheKey = query || '__empty__';
            let files: string[];
            if (lastFileSearchRef.current.query === cacheKey) {
              files = lastFileSearchRef.current.results;
            } else {
              const response = await searchWorkspaceFiles(client, sid, query || '', 20);
              files = response.files || [];
              lastFileSearchRef.current = { query: cacheKey, results: files };
            }
            for (const filePath of files) {
              allItems.push({
                id: filePath,
                kind: 'file',
                label: filePath,
                description: 'File',
                onSelect: () => {},
              });
            }
          }
        } catch {
          // ignore
        }
      }

      return filterItems(allItems, query);
    }, []);

    // ── Suggestion configs ────────────────────────────────────────

    const mentionCallbacks = useMemo((): MentionSuggestionCallbacks => ({
      getItems: getMentionItems,
    }), [getMentionItems]);

    const onAgentCommandRef = useRef(onAgentCommand);
    onAgentCommandRef.current = onAgentCommand;
    const slashCommandsRef = useRef(slashCommands);
    slashCommandsRef.current = slashCommands;

    const slashCallbacks = useMemo((): SlashSuggestionCallbacks => ({
      getCommands: () => slashCommandsRef.current,
      onAgentCommand: (name) => onAgentCommandRef.current?.(name),
    }), []);

    const mentionSuggestion = useMemo(
      () => createMentionSuggestion(mentionCallbacks, setMentionMenu, mentionKeyDownRef),
      [mentionCallbacks],
    );

    const slashSuggestion = useMemo(
      () => createSlashSuggestion(slashCallbacks, setSlashMenu, slashKeyDownRef),
      [slashCallbacks],
    );

    // ── Submit keymap extension ───────────────────────────────────

    const SubmitKeymap = useMemo(() => {
      return Extension.create({
        name: 'submitKeymap',
        addKeyboardShortcuts() {
          return {
            'Enter': () => {
              if (disabledRef.current) return true;
              if (submitKeyRef.current === 'enter') {
                onSubmitRef.current?.();
                return true;
              }
              return false;
            },
            'Mod-Enter': () => {
              if (disabledRef.current) return true;
              if (submitKeyRef.current === 'cmd_enter') {
                onSubmitRef.current?.();
                return true;
              }
              return false;
            },
          };
        },
      });
    }, []);

    // ── Editor ────────────────────────────────────────────────────

    const editor = useEditor({
      immediatelyRender: false,
      extensions: [
        Document,
        Paragraph,
        Text,
        HardBreak,
        History,
        Code,
        CodeBlockLowlight.extend({
          addNodeView() {
            return ReactNodeViewRenderer(CodeBlockView);
          },
        }).configure({ lowlight: lowlightInstance }),
        Placeholder.configure({ placeholder }),
        ContextMention.configure({
          suggestions: [mentionSuggestion, slashSuggestion],
        }),
        SubmitKeymap,
      ],
      editorProps: {
        attributes: {
          class: cn(
            'w-full h-full resize-none bg-transparent px-2 py-2 overflow-y-auto',
            'text-sm leading-relaxed',
            'placeholder:text-muted-foreground',
            'focus:outline-none',
            'disabled:cursor-not-allowed disabled:opacity-50',
            planModeEnabled && 'border-primary/40',
            className,
          ),
        },
        handlePaste: (view, event) => {
          // 1. Image paste
          const items = event.clipboardData?.items;
          if (items) {
            const imageFiles: File[] = [];
            for (const item of items) {
              if (item.type.startsWith('image/')) {
                const file = item.getAsFile();
                if (file) imageFiles.push(file);
              }
            }
            if (imageFiles.length > 0) {
              event.preventDefault();
              onImagePasteRef.current?.(imageFiles);
              return true;
            }
          }

          // 2. Markdown code fence paste
          const text = event.clipboardData?.getData('text/plain');
          if (text && text.includes('```')) {
            const segments = parseCodeFences(text);
            if (segments.some((s) => s.type === 'code')) {
              event.preventDefault();
              const { schema } = view.state;
              const nodes: import('@tiptap/pm/model').Node[] = [];
              for (const seg of segments) {
                if (seg.type === 'code') {
                  nodes.push(
                    schema.nodes.codeBlock.create(
                      seg.language ? { language: seg.language } : null,
                      seg.text ? schema.text(seg.text) : undefined,
                    ),
                  );
                } else {
                  const trimmed = seg.text.trim();
                  if (!trimmed) continue;
                  for (const line of trimmed.split('\n')) {
                    nodes.push(
                      schema.nodes.paragraph.create(
                        null,
                        line ? schema.text(line) : undefined,
                      ),
                    );
                  }
                }
              }
              if (nodes.length > 0) {
                const { from, to } = view.state.selection;
                view.dispatch(view.state.tr.replaceWith(from, to, nodes));
              }
              return true;
            }
          }

          return false;
        },
        handleDOMEvents: {
          focus: () => {
            onFocus?.();
            return false;
          },
          blur: () => {
            onBlur?.();
            return false;
          },
        },
      },
      onUpdate: ({ editor: e }) => {
        if (isSyncingRef.current) return;
        if (!initialSyncDoneRef.current) return;
        const text = getMarkdownText(e);
        onChangeRef.current(text);
        // Persist rich content (mentions, code blocks, etc.)
        const sid = sessionIdRef.current;
        if (sid) setChatDraftContent(sid, e.getJSON());
      },
      editable: !disabled,
    });

    // Guard to prevent circular updates when syncing external value → editor
    const isSyncingRef = useRef(false);
    // Tracks whether the first value→editor sync has completed.
    // Until it has, onUpdate callbacks are suppressed so the editor's
    // init-time empty update doesn't wipe the hydrated draft.
    const initialSyncDoneRef = useRef(false);

    // Sync disabled state
    useEffect(() => {
      if (editor) {
        editor.setEditable(!disabled);
      }
    }, [editor, disabled]);

    // Sync placeholder — the Placeholder extension reads its config once,
    // so we update it when the prop changes.
    useEffect(() => {
      if (!editor) return;
      editor.extensionManager.extensions.forEach((ext) => {
        if (ext.name === 'placeholder') {
          ext.options.placeholder = placeholder;
          editor.view.dispatch(editor.state.tr);
        }
      });
    }, [editor, placeholder]);

    // Reset sync flag when session changes so the new session's content is restored
    const prevSyncSessionRef = useRef(sessionId);
    useEffect(() => {
      if (sessionId === prevSyncSessionRef.current) return;
      prevSyncSessionRef.current = sessionId;
      initialSyncDoneRef.current = false;
    }, [sessionId]);

    // Sync value prop changes (for history navigation, clearing after send)
    // On initial sync, prefer stored JSON (preserves mentions, code blocks) over plain text.
    useEffect(() => {
      /* eslint-disable react-hooks/immutability -- initialSyncDoneRef is intentionally read+written as a one-shot gate */
      if (!editor) return;

      if (!initialSyncDoneRef.current) {
        // Try to restore rich content from sessionStorage
        const sid = sessionId;
        if (sid) {
          const savedContent = getChatDraftContent(sid);
          if (savedContent) {
            isSyncingRef.current = true;
            editor.commands.setContent(savedContent as import('@tiptap/core').Content);
            isSyncingRef.current = false;
            initialSyncDoneRef.current = true;
            // Sync the plain text value back to parent
            onChangeRef.current(getMarkdownText(editor));
            return;
          }
        }
      }

      // When value is empty, always clear — plain-text comparison may miss
      // rich nodes (mentions, code blocks) that have no text representation.
      if (value === '') {
        if (!editor.isEmpty) {
          isSyncingRef.current = true;
          editor.commands.clearContent();
          isSyncingRef.current = false;
        }
        initialSyncDoneRef.current = true;
        return;
      }

      const currentText = getMarkdownText(editor);
      if (currentText === value) {
        initialSyncDoneRef.current = true;
        return;
      }

      isSyncingRef.current = true;
      editor.commands.setContent(`<p>${escapeHtml(value)}</p>`);
      isSyncingRef.current = false;
      initialSyncDoneRef.current = true;
      /* eslint-enable react-hooks/immutability */
    }, [editor, value, sessionId]);

    // ── Imperative handle ─────────────────────────────────────────

    useImperativeHandle(ref, () => ({
      focus: () => editor?.commands.focus(),
      blur: () => editor?.commands.blur(),
      getSelectionStart: () => editor?.state.selection.from ?? 0,
      getValue: () => editor ? getMarkdownText(editor) : '',
      setValue: (v: string) => {
        if (!editor) return;
        isSyncingRef.current = true;
        if (v === '') {
          editor.commands.clearContent();
        } else {
          editor.commands.setContent(`<p>${escapeHtml(v)}</p>`);
        }
        isSyncingRef.current = false;
        onChange(v);
      },
      clear: () => {
        if (!editor) return;
        isSyncingRef.current = true;
        editor.commands.clearContent();
        isSyncingRef.current = false;
        onChange('');
      },
      getTextareaElement: () => editor?.view.dom ?? null,
      insertText: (text: string, from: number, to: number) => {
        if (!editor) return;
        editor.chain().focus().insertContentAt({ from, to }, text).run();
      },
      getMentions: () => {
        if (!editor) return [];
        const mentions: ContextFile[] = [];
        editor.state.doc.descendants((node) => {
          if (node.type.name === 'contextMention') {
            const { kind, path, label } = node.attrs;
            if (kind === 'file') mentions.push({ path, name: label, pinned: false });
            else if (kind === 'prompt') mentions.push({ path, name: label, pinned: false });
            // Skip plan — handled via context store toggle
          }
        });
        return mentions;
      },
    }), [editor, onChange]);

    // ── Menu handlers ─────────────────────────────────────────────

    const handleMentionSelect = useCallback((item: MentionItem) => {
      mentionMenu.command?.(item);
    }, [mentionMenu]);

    const handleMentionClose = useCallback(() => {
      setMentionMenu({ isOpen: false, items: [], query: '', clientRect: null, command: null });
    }, []);

    const handleSlashSelect = useCallback((cmd: SlashCommand) => {
      slashMenu.command?.(cmd);
    }, [slashMenu]);

    const handleSlashClose = useCallback(() => {
      setSlashMenu({ isOpen: false, items: [], query: '', clientRect: null, command: null });
    }, []);

    // ── Render ────────────────────────────────────────────────────

    return (
      <>
        <MentionMenu
          isOpen={mentionMenu.isOpen}
          isLoading={false}
          clientRect={mentionMenu.clientRect}
          items={mentionMenu.items}
          query={mentionMenu.query}
          selectedIndex={mentionSelectedIndex}
          onSelect={handleMentionSelect}
          onClose={handleMentionClose}
          setSelectedIndex={setMentionSelectedIndex}
        />
        <SlashCommandMenu
          isOpen={slashMenu.isOpen}
          clientRect={slashMenu.clientRect}
          commands={slashMenu.items}
          selectedIndex={slashSelectedIndex}
          onSelect={handleSlashSelect}
          onClose={handleSlashClose}
          setSelectedIndex={setSlashSelectedIndex}
        />
        <EditorContextProvider value={{ sessionId, taskId: taskId ?? null }}>
          <EditorContent editor={editor} className="h-full [&_.tiptap]:h-full [&_.tiptap]:outline-none" />
        </EditorContextProvider>
      </>
    );
  },
);

// ── Helpers ─────────────────────────────────────────────────────────

/**
 * Serialize TipTap editor content to markdown-like text.
 * Preserves inline `code`, ```code blocks```, and @mention labels.
 */
function getMarkdownText(editor: { getJSON: () => { content?: JSONNode[] } }): string {
  const doc = editor.getJSON();
  if (!doc.content) return '';
  return doc.content.map(serializeNode).join('\n');
}

type JSONNode = {
  type?: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: Array<{ type: string }>;
  content?: JSONNode[];
};

function serializeNode(node: JSONNode): string {
  switch (node.type) {
    case 'paragraph':
      return serializeInline(node.content ?? []);
    case 'codeBlock': {
      const lang = (node.attrs?.language as string) || '';
      const text = serializeInline(node.content ?? []);
      return '```' + lang + '\n' + text + '\n```';
    }
    case 'hardBreak':
      return '\n';
    default:
      // Unknown block — try to serialize children
      if (node.content) return node.content.map(serializeNode).join('\n');
      return node.text ?? '';
  }
}

function serializeInline(nodes: JSONNode[]): string {
  return nodes.map((n) => {
    if (n.type === 'hardBreak') return '\n';
    if (n.type === 'contextMention') {
      return n.attrs?.label ? `@${n.attrs.label}` : '';
    }
    const text = n.text ?? '';
    if (n.marks?.some((m) => m.type === 'code')) {
      return '`' + text + '`';
    }
    return text;
  }).join('');
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

type FenceSegment = { type: 'text'; text: string } | { type: 'code'; text: string; language: string | null };

/** Parse text containing markdown ``` fences into text/code segments. */
function parseCodeFences(text: string): FenceSegment[] {
  const lines = text.split('\n');
  const segments: FenceSegment[] = [];
  let currentType: 'text' | 'code' = 'text';
  let currentLines: string[] = [];
  let currentLang: string | null = null;

  for (const line of lines) {
    if (line.trimStart().startsWith('```')) {
      if (currentType === 'text') {
        if (currentLines.length > 0) {
          segments.push({ type: 'text', text: currentLines.join('\n') });
        }
        // Extract language from opening fence (e.g. ```typescript)
        currentLang = line.trimStart().slice(3).trim() || null;
        currentLines = [];
        currentType = 'code';
      } else {
        segments.push({ type: 'code', text: currentLines.join('\n'), language: currentLang });
        currentLines = [];
        currentLang = null;
        currentType = 'text';
      }
    } else {
      currentLines.push(line);
    }
  }

  if (currentLines.length > 0) {
    if (currentType === 'code') {
      segments.push({ type: 'code', text: currentLines.join('\n'), language: currentLang });
    } else {
      segments.push({ type: 'text', text: currentLines.join('\n') });
    }
  }

  return segments;
}
