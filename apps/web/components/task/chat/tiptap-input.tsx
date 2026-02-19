"use client";

import {
  forwardRef,
  useRef,
  useCallback,
  useState,
  useEffect,
  useLayoutEffect,
  useMemo,
} from "react";
import { EditorContent } from "@tiptap/react";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { EditorContextProvider } from "./editor-context";
import { MentionMenu } from "./mention-menu";
import { SlashCommandMenu } from "./slash-command-menu";
import {
  createMentionSuggestion,
  createSlashSuggestion,
  type MenuState,
  type MentionSuggestionCallbacks,
  type SlashSuggestionCallbacks,
} from "./tiptap-suggestion";
import { useTipTapEditor, type TipTapInputHandle } from "./use-tiptap-editor";
import type { MentionItem } from "@/hooks/use-inline-mention";
import type { SlashCommand } from "@/hooks/use-inline-slash";
import type { ContextFile } from "@/lib/state/context-files-store";

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

function useMentionItems(sessionId: string | null) {
  const { prompts } = useCustomPrompts();
  const promptsRef = useRef(prompts);
  const sessionIdRef = useRef(sessionId);
  const lastFileSearchRef = useRef<{ query: string; results: string[] }>({
    query: "",
    results: [],
  });
  useLayoutEffect(() => {
    promptsRef.current = prompts;
    sessionIdRef.current = sessionId;
  });

  return useCallback(async (query: string): Promise<MentionItem[]> => {
    const allItems: MentionItem[] = [];
    allItems.push({
      id: "__plan__",
      kind: "plan",
      label: "Plan",
      description: "Include the plan as context",
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
        const client = getWebSocketClient();
        if (client) {
          const cacheKey = query || "__empty__";
          let files: string[];
          if (lastFileSearchRef.current.query === cacheKey) {
            files = lastFileSearchRef.current.results;
          } else {
            const response = await searchWorkspaceFiles(client, sid, query || "", 20);
            files = response.files || [];
            lastFileSearchRef.current = { query: cacheKey, results: files };
          }
          for (const filePath of files) {
            allItems.push({ id: filePath, kind: "file", label: filePath, description: "File", onSelect: () => {} });
          }
        }
      } catch {
        // ignore
      }
    }
    return filterItems(allItems, query);
  }, []);
}

// ── Suggestion configs hook ──────────────────────────────────────────

type SuggestionConfigsInput = {
  sessionId: string | null;
  onAgentCommand?: (commandName: string) => void;
  onMentionKeyDown: (event: KeyboardEvent) => boolean;
  onSlashKeyDown: (event: KeyboardEvent) => boolean;
  setMentionMenu: React.Dispatch<React.SetStateAction<MenuState<MentionItem>>>;
  setSlashMenu: React.Dispatch<React.SetStateAction<MenuState<SlashCommand>>>;
};

function useSuggestionConfigs({
  sessionId,
  onAgentCommand,
  onMentionKeyDown,
  onSlashKeyDown,
  setMentionMenu,
  setSlashMenu,
}: SuggestionConfigsInput) {
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
        description: cmd.description || `Run /${cmd.name} command`,
        action: "agent" as const,
        agentCommandName: cmd.name,
      }));
  }, [agentCommands]);

  const getMentionItems = useMentionItems(sessionId);
  const mentionCallbacks = useMemo(
    (): MentionSuggestionCallbacks => ({ getItems: getMentionItems }),
    [getMentionItems],
  );

  const onAgentCommandRef = useRef(onAgentCommand);
  const slashCommandsRef = useRef(slashCommands);
  useLayoutEffect(() => {
    onAgentCommandRef.current = onAgentCommand;
    slashCommandsRef.current = slashCommands;
  });
  const slashCallbacks = useMemo(
    (): SlashSuggestionCallbacks => ({
      getCommands: () => slashCommandsRef.current,
      onAgentCommand: (name) => onAgentCommandRef.current?.(name),
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

  return { mentionSuggestion, slashSuggestion };
}

// ── Menu state hook ──────────────────────────────────────────────────

function useMenuHandlers() {
  const [mentionMenu, setMentionMenu] = useState<MenuState<MentionItem>>({
    isOpen: false, items: [], query: "", clientRect: null, command: null,
  });
  const [slashMenu, setSlashMenu] = useState<MenuState<SlashCommand>>({
    isOpen: false, items: [], query: "", clientRect: null, command: null,
  });
  const [mentionSelectedIndex, setMentionSelectedIndex] = useState(0);
  const [slashSelectedIndex, setSlashSelectedIndex] = useState(0);

  useEffect(() => { void Promise.resolve().then(() => setMentionSelectedIndex(0)); }, [mentionMenu.items]);
  useEffect(() => { void Promise.resolve().then(() => setSlashSelectedIndex(0)); }, [slashMenu.items]);

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
    (item: MentionItem) => { mentionMenu.command?.(item); },
    [mentionMenu],
  );
  const handleMentionClose = useCallback(
    () => setMentionMenu({ isOpen: false, items: [], query: "", clientRect: null, command: null }),
    [],
  );
  const handleSlashSelect = useCallback(
    (cmd: SlashCommand) => { slashMenu.command?.(cmd); },
    [slashMenu],
  );
  const handleSlashClose = useCallback(
    () => setSlashMenu({ isOpen: false, items: [], query: "", clientRect: null, command: null }),
    [],
  );

  return {
    mentionMenu, setMentionMenu, slashMenu, setSlashMenu,
    mentionSelectedIndex, setMentionSelectedIndex,
    slashSelectedIndex, setSlashSelectedIndex,
    onMentionKeyDown, onSlashKeyDown,
    handleMentionSelect, handleMentionClose,
    handleSlashSelect, handleSlashClose,
  };
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
    submitKey = "cmd_enter",
    onFocus,
    onBlur,
    sessionId,
    taskId,
    onAgentCommand,
    onImagePaste,
  },
  ref,
) {
  const {
    mentionMenu, setMentionMenu, slashMenu, setSlashMenu,
    mentionSelectedIndex, setMentionSelectedIndex,
    slashSelectedIndex, setSlashSelectedIndex,
    onMentionKeyDown, onSlashKeyDown,
    handleMentionSelect, handleMentionClose,
    handleSlashSelect, handleSlashClose,
  } = useMenuHandlers();

  const { mentionSuggestion, slashSuggestion } = useSuggestionConfigs({
    sessionId,
    onAgentCommand,
    onMentionKeyDown,
    onSlashKeyDown,
    setMentionMenu,
    setSlashMenu,
  });

  const editor = useTipTapEditor({
    value, onChange, onSubmit, placeholder, disabled, className,
    planModeEnabled, submitKey, onFocus, onBlur, sessionId, onImagePaste,
    mentionSuggestion, slashSuggestion, ref,
  });

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
        <EditorContent
          editor={editor}
          className="h-full [&_.tiptap]:h-full [&_.tiptap]:outline-none"
        />
      </EditorContextProvider>
    </>
  );
});
