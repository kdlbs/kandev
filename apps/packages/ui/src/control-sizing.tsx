import { cn } from "./lib/utils";

export const CONTROL_SIZING = {
  standard: "h-7 max-md:h-11 [@media(pointer:coarse)]:h-11",
  compact: "h-6",
  icon: "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11",
} as const;

export type ControlSize = keyof typeof CONTROL_SIZING;

export function controlSizingClassName(size: ControlSize, className?: string): string {
  return cn(CONTROL_SIZING[size], className);
}
