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
import { TipTapInputSurface } from "./tiptap-input-surface";
import { handleMenuKeyDown } from "./tiptap-menu-keyboard";
import { exitSuggestion } from "@tiptap/suggestion";
import type { Editor } from "@tiptap/core";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { loadTaskMentionItems } from "./task-mention-items";
import { useChatHistory } from "./tiptap-message-history";
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
import { mapAvailableCommandToSlashCommand, type SlashCommand } from "./slash-command-types";
import type { ContextFile } from "@/lib/state/context-files-store";
import { useEntityReferenceComposer } from "./use-entity-reference-composer";
import { EntityReferenceSuggestionPluginKey } from "./tiptap-entity-reference-suggestion";
import type { ImagePasteIssue } from "./clipboard-attachments";
import { useTranslation } from "react-i18next";
import { useShallow } from "zustand/react/shallow";
import { rankMentionItems, recordChatMentionSelection } from "@/lib/chat-mention-recency";
import { createComposerEscapeCaptureHandler } from "./chat-escape-cancel";

const RAW_DRAIN = { rawPagination: true } as const;

export type { TipTapInputHandle } from "./use-tiptap-editor";

// ── Props ───────────────────────────────────────────────────────────

type TipTapInputProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  onEscape?: (event: React.KeyboardEvent<HTMLElement>) => void;
  shouldHandleEscape?: (event: KeyboardEvent) => boolean;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  planModeEnabled?: boolean;
  submitKey?: "enter" | "cmd_enter";
  onFocus?: () => void;
  onBlur?: () => void;
  // TipTap-specific
  sessionId: string | null;
  detailActive?: boolean;
  taskId?: string | null;
  workspaceId?: string | null;
  entityReferencesEnabled?: boolean;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled?: boolean;
  onImagePaste?: (files: File[], issue?: ImagePasteIssue) => void;
  onPlanModeChange?: (enabled: boolean) => void;
  /** Next-prompt suggestion rendered as ghost text while the draft is empty. */
  promptSuggestion?: string | null;
  onPromptSuggestionDismiss?: () => void;
};

// ── Mention items fetcher hook ───────────────────────────────────────

type FileSearchOwner = {
  taskController?: AbortController;
  sessionId: string | null;
  lookup: number;
  completed: { sessionId: string; query: string; results: string[] } | null;
};

async function fetchFileResults(
  sessionId: string,
  query: string,
  owner: FileSearchOwner,
  isCurrent: () => boolean,
  signal: AbortSignal,
): Promise<string[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const cached = owner.completed;
  if (cached?.sessionId === sessionId && cached.query === query) return cached.results;
  const response = await searchWorkspaceFiles(client, sessionId, query || "", 20, signal);
  if (!isCurrent()) return [];
  owner.completed = { sessionId, query, results: response.files || [] };
  return owner.completed.results;
}

function useMentionItems(
  sessionId: string | null,
  taskId: string | null,
  workspaceId: string | null,
) {
  const { t } = useTranslation();
  const { prompts } = useCustomPrompts();
  const storeApi = useAppStoreApi();
  const promptsRef = useRef(prompts);
  const taskIdRef = useRef(taskId);
  const workspaceIdRef = useRef(workspaceId);
  const lastFileSearchRef = useRef<FileSearchOwner | null>(null);
  useLayoutEffect(() => {
    lastFileSearchRef.current = { sessionId, lookup: 0, completed: null };
    return () => {
      lastFileSearchRef.current?.taskController?.abort();
      lastFileSearchRef.current = null;
    };
  }, [sessionId, taskId, workspaceId]);
  useLayoutEffect(() => {
    promptsRef.current = prompts;
    taskIdRef.current = taskId;
    workspaceIdRef.current = workspaceId;
  });

  return useCallback(
    async (query: string): Promise<MentionItem[]> => {
      const workspace = workspaceIdRef.current;
      const owner = lastFileSearchRef.current;
      if (!owner) return [];
      const lookup = ++owner.lookup;
      owner.taskController?.abort();
      const controller = new AbortController();
      owner.taskController = controller;
      const scope = storeReadScopeIdentity(storeApi);
      const isCurrent = () =>
        lastFileSearchRef.current === owner &&
        owner.lookup === lookup &&
        storeReadScopeIdentity(storeApi) === scope;
      const [allItems, files] = await Promise.all([
        loadTaskMentionItems(storeApi.getState(), taskIdRef.current, workspace, query, {
          signal: controller.signal,
          isCurrent,
        }),
        owner.sessionId
          ? fetchFileResults(owner.sessionId, query, owner, isCurrent, controller.signal).catch(
              () => [],
            )
          : Promise.resolve([]),
      ]);
      if (!isCurrent()) return [];
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
      for (const filePath of files) {
        allItems.push({
          id: filePath,
          kind: "file",
          label: filePath,
          description: t("task:file"),
          onSelect: () => {},
        });
      }
      return isCurrent() ? rankMentionItems(allItems, query, workspace) : [];
    },
    [storeApi],
  );
}

// ── Suggestion configs hook ──────────────────────────────────────────

type SuggestionConfigsInput = {
  sessionId: string | null;
  taskId: string | null;
  workspaceId: string | null;
  onMentionKeyDown: (event: KeyboardEvent) => boolean;
  onSlashKeyDown: (event: KeyboardEvent) => boolean;
  setMentionMenu: React.Dispatch<React.SetStateAction<MenuState<MentionItem>>>;
  setSlashMenu: React.Dispatch<React.SetStateAction<MenuState<SlashCommand>>>;
};

function useSuggestionConfigs({
  sessionId,
  taskId,
  workspaceId,
  onMentionKeyDown,
  onSlashKeyDown,
  setMentionMenu,
  setSlashMenu,
}: SuggestionConfigsInput) {
  const { t } = useTranslation();
  const agentCommands = useAppStore((state) =>
    sessionId ? state.availableCommands.bySessionId[sessionId] : undefined,
  );
  const confirmedConfigOptions = useAppStore(
    useShallow((state) =>
      sessionId ? state.sessionModels.bySessionId[sessionId]?.confirmedConfigOptions : undefined,
    ),
  );
  const slashCommands = useMemo((): SlashCommand[] => {
    if (!agentCommands || agentCommands.length === 0) return [];
    return agentCommands
      .filter((cmd) => !(cmd.description || "").includes("(bundled)"))
      .map((cmd) =>
        mapAvailableCommandToSlashCommand(
          cmd,
          confirmedConfigOptions,
          cmd.description || t("task:runCommand", { name: cmd.name }),
        ),
      );
  }, [agentCommands, confirmedConfigOptions, t]);

  const workspaceIdRef = useRef(workspaceId);
  useLayoutEffect(() => {
    workspaceIdRef.current = workspaceId;
  });

  const getMentionItems = useMentionItems(sessionId, taskId, workspaceId);
  const mentionCallbacks = useMemo(
    (): MentionSuggestionCallbacks => ({
      getItems: getMentionItems,
      onSelect: (item) => recordChatMentionSelection(item, workspaceIdRef.current),
    }),
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
// ── Component ───────────────────────────────────────────────────────

function useComposerMenus(
  sessionId: string | null,
  taskId: string | null,
  workspaceId: string | null,
) {
  const menu = useMenuHandlers();
  const configs = useSuggestionConfigs({
    sessionId,
    taskId,
    workspaceId,
    onMentionKeyDown: menu.onMentionKeyDown,
    onSlashKeyDown: menu.onSlashKeyDown,
    setMentionMenu: menu.setMentionMenu,
    setSlashMenu: menu.setSlashMenu,
  });
  return { menu, ...configs };
}

function useChatComposerInputSurface(
  sessionId: string | null,
  taskId: string | null,
  workspaceId: string | null,
  entityReferencesEnabled: boolean,
) {
  const { menu, mentionSuggestion, slashSuggestion, slashCommands } = useComposerMenus(
    sessionId,
    taskId,
    workspaceId,
  );
  const entityReferences = useEntityReferenceComposer({
    enabled: entityReferencesEnabled,
    workspaceId,
    sessionId,
  });
  const { editorWrapperRef, editorRef, ...overlay } = useReverseSearchOverlay(sessionId);
  const { isSuggestionMenuOpen, closeEntityReferenceMenu } = useSuggestionMenuOpenState(
    menu,
    entityReferences,
    editorWrapperRef,
    editorRef,
  );
  const { history, getHistory } = useChatHistory(sessionId);
  const { isDraining } = useDrainOlderMessages(sessionId, overlay.isReverseSearchOpen, RAW_DRAIN);
  return {
    menu,
    mentionSuggestion,
    slashSuggestion,
    slashCommands,
    entityReferences,
    editorWrapperRef,
    editorRef,
    overlay,
    isSuggestionMenuOpen,
    closeEntityReferenceMenu,
    history,
    getHistory,
    isDraining,
  };
}

export const TipTapInput = forwardRef<TipTapInputHandle, TipTapInputProps>(function TipTapInput(
  {
    value,
    onChange,
    onSubmit,
    onEscape,
    shouldHandleEscape,
    placeholder = "",
    disabled = false,
    className,
    planModeEnabled = false,
    onPlanModeChange,
    submitKey = "cmd_enter",
    onFocus,
    onBlur,
    sessionId,
    detailActive = true,
    taskId,
    workspaceId = null,
    entityReferencesEnabled = false,
    onImagePaste,
    promptSuggestion: suggestion = null,
    onPromptSuggestionDismiss: dismissSuggestion,
  },
  ref,
) {
  const inputSurface = useChatComposerInputSurface(
    sessionId,
    taskId ?? null,
    workspaceId,
    entityReferencesEnabled,
  );
  const {
    mentionSuggestion,
    slashSuggestion,
    slashCommands,
    entityReferences,
    overlay,
    isSuggestionMenuOpen,
    closeEntityReferenceMenu,
    getHistory,
  } = inputSurface;
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
  return (
    <TipTapInputSurface
      {...inputSurface}
      onEntityReferenceClose={closeEntityReferenceMenu}
      editor={editor}
      sessionId={sessionId}
      taskId={taskId ?? null}
      detailActive={detailActive}
      promptSuggestion={suggestion}
      onPromptSuggestionDismiss={dismissSuggestion}
      applyHistoryEntry={applyHistoryEntry}
      escapeGuard={{
        shouldHandleEscape,
        isPromptSuggestionVisible: Boolean(suggestion && !value.trim()),
      }}
      onKeyDownCapture={createComposerEscapeCaptureHandler(
        onEscape,
        isSuggestionMenuOpen,
        overlay.isReverseSearchOpen,
        Boolean(suggestion && !value.trim()),
      )}
    />
  );
});

export type MenuHandlers = ReturnType<typeof useMenuHandlers>;
export type ReverseSearchOverlay = Omit<
  ReturnType<typeof useReverseSearchOverlay>,
  "editorWrapperRef" | "editorRef"
>;

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
  const editorRef = useRef<Editor | null>(null);
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
    // Keep Quick Chat's portal inside its dialog focus scope. Task chat has no
    // dialog and can use document.body.
    setReverseSearchContainer(
      wrapper?.closest<HTMLElement>('[data-slot="dialog-content"]') ?? document.body,
    );
    setIsReverseSearchOpen(true);
  }, []);
  const closeReverseSearch = useCallback(() => setIsReverseSearchOpen(false), []);
  const closeReverseSearchAndFocusEditor = useCallback(() => {
    closeReverseSearch();
    editorRef.current?.commands.focus();
  }, [closeReverseSearch]);
  // Claim Escape before Radix handles it so Quick Chat stays open while the
  // history overlay dismisses itself.
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
    editorRef,
    reverseSearchAnchor,
    reverseSearchContainer,
    isReverseSearchOpen,
    openReverseSearch,
    closeReverseSearch,
    closeReverseSearchAndFocusEditor,
  };
}
