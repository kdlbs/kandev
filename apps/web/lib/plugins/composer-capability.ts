import { useEffect, useMemo, useRef } from "react";
import type { PluginComposerCapability } from "./types";

type NativeComposerOperations = {
  insertText(text: string): boolean;
  focus(): boolean;
  submit(): Promise<boolean>;
};

export type PluginComposerCapabilityHandle = {
  api: PluginComposerCapability;
  /**
   * Marks the capability usable and returns the matching teardown. Wire this
   * as an effect body (`useEffect(() => handle.attach(), [handle])`) rather
   * than calling `revoke` from a cleanup directly: React StrictMode mounts,
   * unmounts and remounts the same component instance, so a cleanup-only
   * revocation would permanently kill a handle whose composer is still on
   * screen.
   */
  attach(): () => void;
  revoke(): void;
};

export function createPluginComposerCapability(
  operations: NativeComposerOperations,
): PluginComposerCapabilityHandle {
  let active = true;

  const revoke = () => {
    active = false;
  };

  return {
    api: {
      insertText(text) {
        if (!active) return { status: "unavailable" };
        if (!text.trim()) return { status: "ignored" };
        return { status: operations.insertText(text) ? "inserted" : "unavailable" };
      },
      focus() {
        if (!active) return { status: "unavailable" };
        return { status: operations.focus() ? "focused" : "unavailable" };
      },
      async submit() {
        if (!active) return { status: "unavailable" };
        return (await operations.submit()) ? { status: "submitted" } : { status: "blocked" };
      },
    },
    attach() {
      active = true;
      return revoke;
    },
    revoke,
  };
}

/**
 * Word-boundary spacing rule every composer adapter applies to an inserted
 * transcript, so the TipTap chat composer and the plain-textarea creation
 * composers behave identically: the text is trimmed, and a single space is
 * added when the character immediately before the insertion point is not
 * already whitespace. Returns "" for an insertion the caller should ignore.
 */
export function composerInsertionText(text: string, charBefore: string): string {
  const trimmed = text.trim();
  if (!trimmed) return "";
  const needsLeadingSpace = charBefore !== "" && !/\s/.test(charBefore);
  return needsLeadingSpace ? ` ${trimmed}` : trimmed;
}

/**
 * Builds the `PluginComposerCapability` a mounted composer hands to its
 * plugin action slot.
 *
 * The returned object is stable for the lifetime of the mount. That is the
 * whole point: a plugin action is inherently asynchronous (record, upload,
 * transcribe, then insert and maybe submit), so it holds the capability
 * across many host re-renders. Rebuilding it whenever `disabled`,
 * `submittable` or the native submit callback changed would revoke the object
 * the plugin is still holding and turn a completed transcription into
 * `unavailable`.
 *
 * Liveness comes from the operations ref instead: each render republishes the
 * current closures, so `submit()` revalidates the native gate at call time
 * rather than against a snapshot taken when the plugin first got the object.
 */
export function useStablePluginComposerCapability(
  operations: NativeComposerOperations,
): PluginComposerCapability {
  const latest = useRef(operations);
  useEffect(() => {
    latest.current = operations;
  });

  const handle = useMemo(
    () =>
      createPluginComposerCapability({
        insertText: (text) => latest.current.insertText(text),
        focus: () => latest.current.focus(),
        submit: () => latest.current.submit(),
      }),
    [],
  );
  useEffect(() => handle.attach(), [handle]);

  return handle.api;
}
