import type { MouseEvent } from "react";

export const TASK_CONFIRM_CLASS = "font-sans !text-sm";
export const stopDialogPropagation = (event: MouseEvent<HTMLDivElement>) => event.stopPropagation();
