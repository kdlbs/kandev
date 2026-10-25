import { useEffect, useState } from "react";
import type { CommandItem, CommandPanelMode } from "@/lib/commands/types";

type Frame = { id: string; search: string; selection: string };
type Options = {
  commands: CommandItem[];
  open: boolean;
  contextKey: string;
  mode: CommandPanelMode;
  setMode: (mode: CommandPanelMode) => void;
  search: string;
  setSearch: (search: string) => void;
  selection: string;
  setSelection: (selection: string) => void;
};

export function useCommandChildren(options: Options) {
  const { commands, open, contextKey, mode, setMode, search, setSearch, selection, setSelection } =
    options;
  const [navigation, setNavigation] = useState<{ contextKey: string; frames: Frame[] }>({
    contextKey,
    frames: [],
  });
  const frames = navigation.contextKey === contextKey ? navigation.frames : [];
  let items = commands;
  let parent: CommandItem | undefined;
  for (const frame of frames) {
    parent = items.find((item) => item.id === frame.id && !item.disabled);
    items = parent?.children ?? [];
  }
  const nested = mode === "command-children";
  useEffect(() => {
    if (open && navigation.contextKey === contextKey && (!nested || parent?.children)) return;
    queueMicrotask(() => {
      setNavigation({ contextKey, frames: [] });
      if (nested) {
        setMode("commands");
        setSearch("");
        setSelection("");
      }
    });
  }, [
    open,
    contextKey,
    navigation.contextKey,
    nested,
    parent?.children,
    setMode,
    setSearch,
    setSelection,
  ]);

  const enter = (command: CommandItem) => {
    setNavigation({
      contextKey,
      frames: [...(nested ? frames : []), { id: command.id, search, selection }],
    });
    setSearch("");
    setSelection(command.children?.find((item) => !item.disabled)?.id ?? "");
    setMode("command-children");
  };
  const back = () => {
    const frame = frames.at(-1);
    setNavigation({ contextKey, frames: frames.slice(0, -1) });
    setSearch(frame?.search ?? "");
    setSelection(frame?.selection ?? "");
    setMode(frames.length > 1 ? "command-children" : "commands");
  };
  return { commands: nested ? items : commands, parent: nested ? parent : undefined, enter, back };
}
