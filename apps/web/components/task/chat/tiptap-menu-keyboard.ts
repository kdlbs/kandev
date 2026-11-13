import type { Dispatch, RefObject, SetStateAction } from "react";
import type { MenuState } from "./tiptap-suggestion";

export function handleMenuKeyDown<T>(
  event: KeyboardEvent,
  menu: MenuState<T>,
  setIndex: Dispatch<SetStateAction<number>>,
  indexRef: RefObject<number>,
): boolean {
  if (!menu.isOpen) return false;
  if (event.key === "ArrowDown") {
    setIndex((index) => Math.min(index + 1, menu.items.length - 1));
    return true;
  }
  if (event.key === "ArrowUp") {
    setIndex((index) => Math.max(index - 1, 0));
    return true;
  }
  if ((event.key === "Enter" || event.key === "Tab") && menu.items.length > 0 && menu.command) {
    const item = menu.items[indexRef.current];
    if (item) menu.command(item);
    return true;
  }
  return false;
}
