"use client";

import {
  forwardRef,
  useRef,
  useCallback,
  useState,
  useEffect,
  useLayoutEffect,
  useMemo,
  type RefObject,
} from "react";
import { EditorContent } from "@tiptap/react";
import { exitSuggestion } from "@tiptap/suggestion";
import type { Editor } from "@tiptap/core";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { EditorContextProvider } from "./editor-context";
import { EntityReferenceMenu } from "./entity-reference-menu";
import { MentionMenu } from "./mention-menu";
import { MessageHistorySearch } from "./message-history-search";
import { SlashCommandMenu } from "./slash-command-menu";
import { buildTaskMentionItems } from "./task-mention-items";
import { createMessageHistorySelector, type MessageHistoryEntry } from "./message-history";
import { useDrainOlderMessages } from "./use-drain-older-messages";
import {
  createMentionSuggestion,
  createSlashSuggestion,
  type MenuState,
  type MentionSuggestionCallbacks,
  type SlashSuggestionCallbacks,
} from "./tiptap-suggestion";
import { useTipTapEditor, type TipTapInputHandle } from "./use-tiptap-editor";
import { useSuggestionEscapeFallback } from "./use-suggestion-escape-fallback";
import { getSuggestionMenuOpenState } from "./suggestion-menu-state";
import {
  useClarificationEscapeGuard,
  type ClarificationEscapePredicate,
} from "@/hooks/use-clarification-escape-guard";
import type { MentionItem } from "@/hooks/use-inline-mention";
import type { SlashCommand } from "./slash-command-types";
import type { ContextFile } from "@/lib/state/context-files-store";
import { useEntityReferenceComposer } from "./use-entity-reference-composer";
import { EntityReferenceSuggestionPluginKey } from "./tiptap-entity-reference-suggestion";
import type { ImagePasteIssue } from "./clipboard-attachments";
import { useTranslation } from "react-i18next";

const RAW_DRAIN = { rawPagination: true } as const;

export type { TipTapInputHandle } from "./use-tiptap-editor";

// ── Props ───────────────────────────────────────────────────────────

type TipTapInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  planModeEnabled?: boolean;
  submitKey?: "enter" | "cmd_enter";
  onFocus?: () => void;
  onBlur?: () => void;
  // TipTap-specific
  sessionId: string | null;
  taskId?: string | null;
  workspaceId?: string | null;
  entityReferencesEnabled?: boolean;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled?: boolean;
  onImagePaste?: (files: File[], issue?: ImagePasteIssue) => void;
  onPlanModeChange?: (enabled: boolean) => void;
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

// ── Menu keyboard navigation helper ──────────────────────────────

function handleMenuKeyDown<T>(
  event: KeyboardEvent,
  menu: MenuState<T>,
  setIndex: React.Dispatch<React.SetStateAction<number>>,
  indexRef: React.RefObject<number>,
): boolean {
  if (!menu.isOpen) return false;
  if (event.key === "ArrowDown") {
    setIndex((i) => Math.min(i + 1, menu.items.length - 1));
    return true;
  }
  if (event.key === "ArrowUp") {
    setIndex((i) => Math.max(i - 1, 0));
    return true;
  }
  if (event.key === "Enter" || event.key === "Tab") {
    if (menu.items.length > 0 && menu.command) {
      const item = menu.items[indexRef.current];
      if (item) menu.command(item);
      return true;
    }
  }
  return false;
}

// ── Mention items fetcher hook ───────────────────────────────────────

async function fetchFileResults(
  sessionId: string,
  query: string,
  cache: { query: string; results: string[] },
): Promise<string[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const cacheKey = query || "__empty__";
  if (cache.query === cacheKey) return cache.results;
  const response = await searchWorkspaceFiles(client, sessionId, query || "", 20);
  const results = response.files || [];
  cache.query = cacheKey;
  cache.results = results;
  return results;
}

function useMentionItems(sessionId: string | null, taskId: string | null) {
  const { t } = useTranslation();
  const { prompts } = useCustomPrompts();
  const storeApi = useAppStoreApi();
  const promptsRef = useRef(prompts);
  const sessionIdRef = useRef(sessionId);
  const taskIdRef = useRef(taskId);
  const lastFileSearchRef = useRef<{ query: string; results: string[] }>({
    query: "",
    results: [],
  });
  useLayoutEffect(() => {
    promptsRef.current = prompts;
    sessionIdRef.current = sessionId;
    taskIdRef.current = taskId;
  });

  return useCallback(
    async (query: string): Promise<MentionItem[]> => {
      const allItems: MentionItem[] = [];
      allItems.push(...buildTaskMentionItems(storeApi.getState(), taskIdRef.current));
      allItems.push({
        id: "__plan__",
        kind: "plan",
        label: t("task:plan"),
        description: t("task:includeThePlanAsContext"),
        onSelect: () => {},
      });
      for (const p of promptsRef.current) {
        allItems.push({
          id: p.id,
          kind: "prompt",
          label: p.name,
          description: p.content.length > 100 ? p.content.slice(0, 100) + "..." : p.content,
          onSelect: () => {},
        });
      }
      const sid = sessionIdRef.current;
      if (sid) {
        try {
          const files = await fetchFileResults(sid, query, lastFileSearchRef.current);
          for (const filePath of files) {
            allItems.push({
              id: filePath,
              kind: "file",
              label: filePath,
              description: t("task:file"),
              onSelect: () => {},
            });
          }
        } catch {
          // ignore
        }
      }
      return filterItems(allItems, query);
    },
    [storeApi],
  );
}

// ── Suggestion configs hook ──────────────────────────────────────────

type SuggestionConfigsInput = {
  sessionId: string | null;
  taskId: string | null;
  onMentionKeyDown: (event: KeyboardEvent) => boolean;
  onSlashKeyDown: (event: KeyboardEvent) => boolean;
  setMentionMenu: React.Dispatch<React.SetStateAction<MenuState<MentionItem>>>;
  setSlashMenu: React.Dispatch<React.SetStateAction<MenuState<SlashCommand>>>;
};

function useSuggestionConfigs({
  sessionId,
  taskId,
  onMentionKeyDown,
  onSlashKeyDown,
  setMentionMenu,
  setSlashMenu,
}: SuggestionConfigsInput) {
  const { t } = useTranslation();
  const agentCommands = useAppStore((state) =>
    sessionId ? state.availableCommands.bySessionId[sessionId] : undefined,
  );
  const slashCommands = useMemo((): SlashCommand[] => {
    if (!agentCommands || agentCommands.length === 0) return [];
    return agentCommands
      .filter((cmd) => !(cmd.description || "").includes("(bundled)"))
      .map((cmd) => ({
        id: `agent-${cmd.name}`,
        label: `/${cmd.name}`,
        description: cmd.description || t("task:runCommand", { name: cmd.name }),
        action: "agent" as const,
        agentCommandName: cmd.name,
      }));
  }, [agentCommands]);

  const getMentionItems = useMentionItems(sessionId, taskId);
  const mentionCallbacks = useMemo(
    (): MentionSuggestionCallbacks => ({ getItems: getMentionItems }),
    [getMentionItems],
  );

  const slashCommandsRef = useRef(slashCommands);
  useLayoutEffect(() => {
    slashCommandsRef.current = slashCommands;
  });
  const slashCallbacks = useMemo(
    (): SlashSuggestionCallbacks => ({
      getCommands: () => slashCommandsRef.current,
    }),
    [],
  );

  /* eslint-disable react-hooks/refs -- mentionCallbacks/slashCallbacks capture refs for deferred access, not during render */
  const mentionSuggestion = useMemo(
    () => createMentionSuggestion(mentionCallbacks, setMentionMenu, onMentionKeyDown),
    [mentionCallbacks, setMentionMenu, onMentionKeyDown],
  );
  const slashSuggestion = useMemo(
    () => createSlashSuggestion(slashCallbacks, setSlashMenu, onSlashKeyDown),
    [slashCallbacks, setSlashMenu, onSlashKeyDown],
  );
  /* eslint-enable react-hooks/refs */

  return { mentionSuggestion, slashSuggestion, slashCommands };
}

// ── Menu state hook ──────────────────────────────────────────────────

function useMenuHandlers() {
  const [mentionMenu, setMentionMenu] = useState<MenuState<MentionItem>>({
    isOpen: false,
    items: [],
    query: "",
    clientRect: null,
    command: null,
  });
  const [slashMenu, setSlashMenu] = useState<MenuState<SlashCommand>>({
    isOpen: false,
    items: [],
    query: "",
    clientRect: null,
    command: null,
  });
  const [mentionSelectedIndex, setMentionSelectedIndex] = useState(0);
  const [slashSelectedIndex, setSlashSelectedIndex] = useState(0);

  useEffect(() => {
    void Promise.resolve().then(() => setMentionSelectedIndex(0));
  }, [mentionMenu.items]);
  useEffect(() => {
    void Promise.resolve().then(() => setSlashSelectedIndex(0));
  }, [slashMenu.items]);

  const mentionSelectedIndexRef = useRef(mentionSelectedIndex);
  const slashSelectedIndexRef = useRef(slashSelectedIndex);
  const mentionKeyDownRef = useRef<((event: KeyboardEvent) => boolean) | null>(null);
  const slashKeyDownRef = useRef<((event: KeyboardEvent) => boolean) | null>(null);

  const mentionKeyDown = useCallback(
    (event: KeyboardEvent) =>
      handleMenuKeyDown(event, mentionMenu, setMentionSelectedIndex, mentionSelectedIndexRef),
    [mentionMenu],
  );
  const slashKeyDown = useCallback(
    (event: KeyboardEvent) =>
      handleMenuKeyDown(event, slashMenu, setSlashSelectedIndex, slashSelectedIndexRef),
    [slashMenu],
  );
  const onMentionKeyDown = useCallback(
    (event: KeyboardEvent) => mentionKeyDownRef.current?.(event) ?? false,
    [],
  );
  const onSlashKeyDown = useCallback(
    (event: KeyboardEvent) => slashKeyDownRef.current?.(event) ?? false,
    [],
  );

  useLayoutEffect(() => {
    mentionSelectedIndexRef.current = mentionSelectedIndex;
    slashSelectedIndexRef.current = slashSelectedIndex;
    mentionKeyDownRef.current = mentionKeyDown;
    slashKeyDownRef.current = slashKeyDown;
  });

  const handleMentionSelect = useCallback(
    (item: MentionItem) => {
      mentionMenu.command?.(item);
    },
    [mentionMenu],
  );
  const handleMentionClose = useCallback(
    () => setMentionMenu({ isOpen: false, items: [], query: "", clientRect: null, command: null }),
    [],
  );
  const handleSlashSelect = useCallback(
    (cmd: SlashCommand) => {
      slashMenu.command?.(cmd);
    },
    [slashMenu],
  );
  const handleSlashClose = useCallback(
    () => setSlashMenu({ isOpen: false, items: [], query: "", clientRect: null, command: null }),
    [],
  );

  return {
    mentionMenu,
    setMentionMenu,
    slashMenu,
    setSlashMenu,
    mentionSelectedIndex,
    setMentionSelectedIndex,
    slashSelectedIndex,
    setSlashSelectedIndex,
    onMentionKeyDown,
    onSlashKeyDown,
    handleMentionSelect,
    handleMentionClose,
    handleSlashSelect,
    handleSlashClose,
  };
}

function useEntityReferenceMenuClose(editorRef: RefObject<Editor | null>, close: () => void) {
  return useCallback(() => {
    const editor = editorRef.current;
    if (editor) {
      exitSuggestion(editor.view, EntityReferenceSuggestionPluginKey);
    }
    close();
  }, [editorRef, close]);
}

// `editorRef` is created before `editor` exists (it backs a close handler
// composed earlier, in useSuggestionMenuOpenState) -- resync it every render
// so callers reading `editorRef.current` always see the latest instance.
function useEditorRefSync(editorRef: RefObject<Editor | null>, editor: Editor | null) {
  useLayoutEffect(() => {
    editorRef.current = editor;
  });
}

function useReverseSearchSelectHandler(
  applyHistoryEntry: (index: number) => void,
  closeReverseSearch: () => void,
  editor: Editor | null,
) {
  return useCallback(
    (index: number) => {
      applyHistoryEntry(index);
      closeReverseSearch();
      editor?.commands.focus("end");
    },
    [applyHistoryEntry, closeReverseSearch, editor],
  );
}

// ── Component ───────────────────────────────────────────────────────

export const TipTapInput = forwardRef<TipTapInputHandle, TipTapInputProps>(function TipTapInput(
  {
    value,
    onChange,
    onSubmit,
    placeholder = "",
    disabled = false,
    className,
    planModeEnabled = false,
    onPlanModeChange,
    submitKey = "cmd_enter",
    onFocus,
    onBlur,
    sessionId,
    taskId,
    workspaceId = null,
    entityReferencesEnabled = false,
    onImagePaste,
  },
  ref,
) {
  const menu = useMenuHandlers();
  const { mentionSuggestion, slashSuggestion, slashCommands } = useSuggestionConfigs({
    sessionId,
    taskId: taskId ?? null,
    onMentionKeyDown: menu.onMentionKeyDown,
    onSlashKeyDown: menu.onSlashKeyDown,
    setMentionMenu: menu.setMentionMenu,
    setSlashMenu: menu.setSlashMenu,
  });
  const entityReferences = useEntityReferenceComposer({
    enabled: entityReferencesEnabled,
    workspaceId,
    sessionId,
  });
  const { editorWrapperRef, ...overlay } = useReverseSearchOverlay(sessionId);
  const editorRef = useRef<Editor | null>(null);
  const { isSuggestionMenuOpen, closeEntityReferenceMenu } = useSuggestionMenuOpenState(
    menu,
    entityReferences,
    editorWrapperRef,
    editorRef,
  );
  const { history, getHistory } = useChatHistory(sessionId);
  const { isDraining } = useDrainOlderMessages(sessionId, overlay.isReverseSearchOpen, RAW_DRAIN);
  const { editor, applyHistoryEntry } = useTipTapEditor({
    value,
    onChange,
    onSubmit,
    placeholder,
    disabled,
    className,
    planModeEnabled,
    onPlanModeChange,
    submitKey,
    onFocus,
    onBlur,
    sessionId,
    onImagePaste,
    mentionSuggestion,
    slashSuggestion,
    entityReferenceSuggestion: entityReferences.suggestion,
    onTextInput: entityReferences.onTextInput,
    onBeforeInput: entityReferences.onBeforeInput,
    slashCommands,
    isSuggestionMenuOpen,
    getHistory,
    onOpenReverseSearch: overlay.openReverseSearch,
    isReverseSearchOpen: overlay.isReverseSearchOpen,
    ref,
  });
  useEditorRefSync(editorRef, editor);
  const { closeReverseSearch } = overlay;
  const handleReverseSearchSelect = useReverseSearchSelectHandler(
    applyHistoryEntry,
    closeReverseSearch,
    editor,
  );
  return (
    <>
      <TipTapPopups
        menu={menu}
        entityReferences={entityReferences}
        overlay={overlay}
        history={history}
        isDraining={isDraining}
        onReverseSearchSelect={handleReverseSearchSelect}
        onEntityReferenceClose={closeEntityReferenceMenu}
      />
      <EditorContextProvider value={{ sessionId, taskId: taskId ?? null }}>
        <div ref={editorWrapperRef} className="h-full">
          <EditorContent
            editor={editor}
            className="h-full [&_.tiptap]:h-full [&_.tiptap]:outline-none"
          />
        </div>
      </EditorContextProvider>
    </>
  );
});

type TipTapPopupsProps = {
  menu: ReturnType<typeof useMenuHandlers>;
  entityReferences: ReturnType<typeof useEntityReferenceComposer>;
  overlay: Omit<ReturnType<typeof useReverseSearchOverlay>, "editorWrapperRef">;
  history: readonly MessageHistoryEntry[];
  isDraining: boolean;
  onReverseSearchSelect: (index: number) => void;
  onEntityReferenceClose: () => void;
};

function TipTapPopups({
  menu,
  entityReferences,
  overlay,
  history,
  isDraining,
  onReverseSearchSelect,
  onEntityReferenceClose,
}: TipTapPopupsProps) {
  return (
    <>
      <MentionMenu
        isOpen={menu.mentionMenu.isOpen}
        isLoading={false}
        clientRect={menu.mentionMenu.clientRect}
        items={menu.mentionMenu.items}
        query={menu.mentionMenu.query}
        selectedIndex={menu.mentionSelectedIndex}
        onSelect={menu.handleMentionSelect}
        onClose={menu.handleMentionClose}
        setSelectedIndex={menu.setMentionSelectedIndex}
      />
      <EntityReferenceMenu
        isOpen={entityReferences.isOpen}
        clientRect={entityReferences.clientRect}
        groups={entityReferences.groups}
        query={entityReferences.query}
        selectedIndex={entityReferences.selectedIndex}
        isSearching={entityReferences.isSearching}
        error={entityReferences.error}
        onRetry={entityReferences.retry}
        onSelect={entityReferences.selectReference}
        onClose={onEntityReferenceClose}
        setSelectedIndex={entityReferences.setSelectedIndex}
      />
      <SlashCommandMenu
        isOpen={menu.slashMenu.isOpen}
        clientRect={menu.slashMenu.clientRect}
        commands={menu.slashMenu.items}
        selectedIndex={menu.slashSelectedIndex}
        onSelect={menu.handleSlashSelect}
        onClose={menu.handleSlashClose}
        setSelectedIndex={menu.setSlashSelectedIndex}
      />
      {overlay.isReverseSearchOpen && overlay.reverseSearchContainer && (
        <MessageHistorySearch
          history={history}
          isLoadingOlder={isDraining}
          anchorRect={overlay.reverseSearchAnchor}
          container={overlay.reverseSearchContainer}
          onClose={overlay.closeReverseSearch}
          onSelect={onReverseSearchSelect}
        />
      )}
    </>
  );
}

function useMessageHistoryForSession(sessionId: string | null): MessageHistoryEntry[] {
  // The memoized selector keeps its snapshot stable while agent messages
  // stream, but still reacts to user content or reference-metadata changes.
  const selector = useMemo(() => createMessageHistorySelector(sessionId), [sessionId]);
  return useAppStore(selector);
}

function useChatHistory(sessionId: string | null) {
  const history = useMessageHistoryForSession(sessionId);
  const historyRef = useRef(history);
  useLayoutEffect(() => {
    historyRef.current = history;
  });
  const getHistory = useCallback(() => historyRef.current, []);
  return { history, getHistory };
}

function useSuggestionMenuOpenState(
  menu: ReturnType<typeof useMenuHandlers>,
  entityReferences: ReturnType<typeof useEntityReferenceComposer>,
  containerRef: RefObject<HTMLElement | null>,
  editorRef: RefObject<Editor | null>,
) {
  const { mentionMenuOpen, slashMenuOpen, isSuggestionMenuOpen } = getSuggestionMenuOpenState({
    mentionIsOpen: menu.mentionMenu.isOpen,
    slashIsOpen: menu.slashMenu.isOpen,
    slashItemCount: menu.slashMenu.items.length,
    entityReferenceMenuOpen: entityReferences.isOpen,
  });
  const closeEntityReferenceMenu = useEntityReferenceMenuClose(editorRef, entityReferences.close);
  useSuggestionEscapeFallback({
    isSuggestionMenuOpen,
    mentionMenuOpen,
    slashMenuOpen,
    entityReferenceMenuOpen: entityReferences.isOpen,
    closeMentionMenu: menu.handleMentionClose,
    closeSlashMenu: menu.handleSlashClose,
    closeEntityReferenceMenu,
    containerRef,
  });
  return { isSuggestionMenuOpen, closeEntityReferenceMenu };
}

// Stable reference (module scope) so the guard registry sees the same
// predicate identity across renders while the overlay stays open, instead of
// re-registering on every render.
const CLAIM_ANY_ESCAPE: ClarificationEscapePredicate = () => true;

function useReverseSearchOverlay(sessionId: string | null) {
  const editorWrapperRef = useRef<HTMLDivElement>(null);
  const [reverseSearchAnchor, setReverseSearchAnchor] = useState<DOMRect | null>(null);
  const [reverseSearchContainer, setReverseSearchContainer] = useState<Element | null>(null);
  const [isReverseSearchOpen, setIsReverseSearchOpen] = useState(false);
  const sessionIdRef = useRef(sessionId);
  useLayoutEffect(() => {
    sessionIdRef.current = sessionId;
  });
  const openReverseSearch = useCallback(() => {
    if (!sessionIdRef.current) return;
    const wrapper = editorWrapperRef.current;
    setReverseSearchAnchor(wrapper?.getBoundingClientRect() ?? null);
    // Radix's Dialog traps focus within [data-slot="dialog-content"]. Portaling
    // outside that scope (the prior document.body default) meant the overlay's
    // own focus() and Escape/typing handlers never fired on a surface that
    // renders this composer inside a Dialog (Quick Chat) -- the trap reverted
    // focus every time. Render inside that scope when one wraps the composer;
    // otherwise (the non-modal main task chat panel) keep document.body.
    setReverseSearchContainer(
      wrapper?.closest<HTMLElement>('[data-slot="dialog-content"]') ?? document.body,
    );
    setIsReverseSearchOpen(true);
  }, []);
  const closeReverseSearch = useCallback(() => setIsReverseSearchOpen(false), []);
  // On Quick Chat, Radix's DismissableLayer dismisses the whole dialog on
  // Escape unless something already called preventDefault() during the same
  // document-capture pass -- see use-suggestion-escape-fallback.ts for the
  // full mechanism. The overlay's own onKeyDown (message-history-search.tsx)
  // runs later, in the bubble phase, too late to stop that. Registering here
  // tells the dialog this Escape is spoken for, so it stays open and lets the
  // overlay's own handler close just the overlay. No-ops on the main task
  // chat panel, where there is no ClarificationEscapeGuardProvider.
  useClarificationEscapeGuard(isReverseSearchOpen ? CLAIM_ANY_ESCAPE : null);
  // The anchor rect is captured once at open time; dismiss on viewport
  // changes rather than recompute, matching how the project's other
  // fixed-position popups behave on resize/scroll. Bubbling-phase scroll
  // only — capture-phase would also catch the overlay's own list scroll
  // (e.g. from scrollIntoView during keyboard navigation), which would
  // dismiss the overlay mid-browse.
  useEffect(() => {
    if (!isReverseSearchOpen) return;
    window.addEventListener("resize", closeReverseSearch);
    window.addEventListener("scroll", closeReverseSearch);
    return () => {
      window.removeEventListener("resize", closeReverseSearch);
      window.removeEventListener("scroll", closeReverseSearch);
    };
  }, [isReverseSearchOpen, closeReverseSearch]);
  return {
    editorWrapperRef,
    reverseSearchAnchor,
    reverseSearchContainer,
    isReverseSearchOpen,
    openReverseSearch,
    closeReverseSearch,
  };
}
