import { cn } from "@/lib/utils";

type PropertyRowProps = {
  label: string;
  children: React.ReactNode;
  /** Extra classes for the row container. */
  className?: string;
  /** Extra classes for the value side (handy for stretchy pickers). */
  valueClassName?: string;
  /** When true, align children to the top instead of centred. */
  alignStart?: boolean;
};

/**
 * Single-line layout used for every row in the right-side Properties
 * panel. The label sits in a fixed-width column on the left and the
 * value occupies the remaining width on the right.
 */
export function PropertyRow({
  label,
  children,
  className,
  valueClassName,
  alignStart = false,
}: PropertyRowProps) {
  return (
    <div
      className={cn(
        "flex justify-between py-2 border-b border-border/50",
        alignStart ? "items-start" : "items-center",
        className,
      )}
    >
      <span className="text-sm text-muted-foreground w-24 shrink-0 pt-0.5">{label}</span>
      <div className={cn("text-sm text-right flex-1 min-w-0", valueClassName)}>{children}</div>
    </div>
  );
}
