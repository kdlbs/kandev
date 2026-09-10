import type { MouseEvent } from "react";

export const TASK_CONFIRM_CLASS =
  "font-sans !text-sm max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden";
export const TASK_CONFIRM_HEADER_CLASS = "place-items-start text-left";
export const TASK_CONFIRM_BODY_CLASS =
  "min-h-0 min-w-0 space-y-3 overflow-y-auto overscroll-contain text-left";
export const TASK_CONFIRM_FOOTER_CLASS = "w-full sm:w-auto";
export const TASK_CONFIRM_ACTION_CLASS =
  "cursor-pointer !text-sm min-h-11 w-full sm:min-h-0 sm:w-auto";
export const stopDialogPropagation = (event: MouseEvent<HTMLDivElement>) => event.stopPropagation();
