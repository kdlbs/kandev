import { cn } from "./lib/utils";

// Keep touch targets at 44 CSS pixels when the root font size is reduced.
export const CONTROL_SIZING = {
  standard:
    "h-7 max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-h-[44px] [@media(any-pointer:coarse)]:min-h-[44px]",
  compact: "h-6",
  icon: "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11 max-md:min-h-[44px] max-md:min-w-[44px] [@media(any-pointer:coarse)]:min-h-[44px] [@media(any-pointer:coarse)]:min-w-[44px]",
} as const;

export type ControlSize = keyof typeof CONTROL_SIZING;

export function controlSizingClassName(size: ControlSize, className?: string): string {
  return cn(CONTROL_SIZING[size], className);
}
