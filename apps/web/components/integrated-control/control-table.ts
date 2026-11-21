import { cn } from "@/lib/utils";

/** Right-aligned fixed-width digits, so a column of numbers lines up. */
export const NUMBER_CELL = "text-right font-mono tabular-nums";

/** A number cell; a zero is dimmed so the non-zero values stand out. */
export function numberCellClass(value: unknown, className?: string) {
  return cn(NUMBER_CELL, value === 0 && "text-muted-foreground/60", className);
}
