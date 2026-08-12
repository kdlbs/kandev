import type { PluginComposerCapability } from "./types";

type NativeComposerOperations = {
  insertText(text: string): boolean;
  focus(): boolean;
  submit(): Promise<boolean>;
};

export function createPluginComposerCapability(operations: NativeComposerOperations): {
  api: PluginComposerCapability;
  revoke(): void;
} {
  let active = true;

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
    revoke() {
      active = false;
    },
  };
}
