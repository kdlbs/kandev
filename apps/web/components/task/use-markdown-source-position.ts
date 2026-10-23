import { useCallback, useLayoutEffect, useMemo, useRef } from "react";
import type { SourcePosition, SourcePositionHandle } from "@/components/editors/source-position";
import { clampSourcePosition } from "./markdown-source-position";
import type { MarkdownFileMode } from "./markdown-file-mode";

export type MarkdownSourcePositionPorts = Record<
  MarkdownFileMode,
  (handle: SourcePositionHandle | null) => void
>;
export type MarkdownSourcePositionOptions = {
  identity: string;
  draftIdentity?: unknown;
  content: string;
  mode?: MarkdownFileMode;
  onModeChange: (mode: MarkdownFileMode) => void;
};

type PositionOwner = {
  identity: string;
  draftIdentity: unknown;
  content: string;
  mode?: MarkdownFileMode;
  live: boolean;
  transition: number;
  handles: Partial<Record<MarkdownFileMode, SourcePositionHandle>>;
  pending?: { target: MarkdownFileMode; position: SourcePosition; transition: number };
};

export function useMarkdownSourcePosition({
  identity,
  draftIdentity,
  content,
  mode,
  onModeChange,
}: MarkdownSourcePositionOptions) {
  const ownerRef = useRef<PositionOwner>({
    identity,
    draftIdentity,
    content,
    mode,
    live: true,
    transition: 0,
    handles: {},
  });
  if (ownerRef.current.identity !== identity || ownerRef.current.draftIdentity !== draftIdentity) {
    ownerRef.current.live = false;
    ownerRef.current = {
      identity,
      draftIdentity,
      content,
      mode,
      live: true,
      transition: 0,
      handles: {},
    };
  }
  const owner = ownerRef.current;
  if (owner.content !== content || (owner.mode !== mode && owner.pending?.target !== mode))
    owner.pending = undefined;
  owner.content = content;
  owner.mode = mode;
  const transition = owner.transition;

  useLayoutEffect(() => {
    owner.live = true;
    return () => {
      owner.live = false;
      owner.pending = undefined;
      owner.handles = {};
    };
  }, [owner]);

  const prepareModeChange = useCallback(
    (next: MarkdownFileMode) => {
      if (!owner.live || ownerRef.current !== owner || next === owner.mode) return;
      const position = owner.mode ? owner.handles[owner.mode]?.capture() : null;
      owner.transition += 1;
      owner.handles = {};
      owner.pending = position
        ? {
            target: next,
            position: clampSourcePosition(owner.content, position),
            transition: owner.transition,
          }
        : undefined;
    },
    [owner],
  );

  const handleModeChange = useCallback(
    (next: MarkdownFileMode) => {
      prepareModeChange(next);
      onModeChange(next);
    },
    [onModeChange, prepareModeChange],
  );

  const sourcePositionHandles = useMemo(() => {
    const register = (target: MarkdownFileMode) => (handle: SourcePositionHandle | null) => {
      if (!owner.live || ownerRef.current !== owner || owner.transition !== transition) return;
      if (handle) owner.handles[target] = handle;
      else delete owner.handles[target];
      const pending = owner.pending;
      if (
        !handle ||
        owner.mode !== target ||
        pending?.target !== target ||
        pending.transition !== transition
      )
        return;
      owner.pending = undefined;
      handle.restore(clampSourcePosition(owner.content, pending.position));
    };
    return { preview: register("preview"), edit: register("edit"), source: register("source") };
  }, [owner, transition]);

  return { handleModeChange, prepareModeChange, sourcePositionHandles };
}
