import { useEffect } from "react";
import type { RefObject } from "react";

/** Preserve selection when a dialog focus trap restores its input after external blur. */
export function useExternalFocusSelection(inputRef: RefObject<HTMLInputElement | null>) {
  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    const doc = input.ownerDocument;
    const win = doc.defaultView!;
    let selection:
      | { start: number; end: number; direction: "forward" | "backward" | "none"; value: string }
      | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const remember = () => {
      selection = {
        start: input.selectionStart ?? 0,
        end: input.selectionEnd ?? 0,
        direction: input.selectionDirection ?? "none",
        value: input.value,
      };
    };
    const windowBlur = () => {
      if (doc.activeElement === input) remember();
    };
    const inputBlur = (event: FocusEvent) => {
      if (!event.relatedTarget && (!doc.hasFocus() || doc.visibilityState === "hidden")) {
        remember();
      }
    };
    const clear = () => {
      clearTimeout(timer);
      selection = undefined;
    };
    const restore = () => {
      const saved = selection;
      if (!saved) return;
      clearTimeout(timer);
      timer = setTimeout(() => {
        if (selection !== saved) return;
        if (doc.activeElement === input && input.value === saved.value) {
          input.setSelectionRange(saved.start, saved.end, saved.direction);
        }
      }, 0);
    };
    win.addEventListener("blur", windowBlur);
    input.addEventListener("blur", inputBlur);
    input.addEventListener("focus", restore);
    doc.addEventListener("pointerdown", clear, true);
    input.addEventListener("input", clear);
    doc.addEventListener("keydown", clear, true);
    return () => {
      clear();
      win.removeEventListener("blur", windowBlur);
      input.removeEventListener("blur", inputBlur);
      input.removeEventListener("focus", restore);
      doc.removeEventListener("pointerdown", clear, true);
      input.removeEventListener("input", clear);
      doc.removeEventListener("keydown", clear, true);
    };
  }, [inputRef]);
}
