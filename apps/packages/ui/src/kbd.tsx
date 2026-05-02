import { cn } from "./lib/utils";

function Kbd({ className, ...props }: React.ComponentProps<"kbd">) {
  return (
    <kbd
      data-slot="kbd"
      className={cn(
        "bg-muted text-muted-foreground [[data-slot=tooltip-content]_&]:border [[data-slot=tooltip-content]_&]:border-border/60 [[data-slot=tooltip-content]_&]:bg-background/70 [[data-slot=tooltip-content]_&]:text-popover-foreground dark:[[data-slot=tooltip-content]_&]:bg-background/20 h-5 w-fit min-w-5 gap-1 rounded-xs px-1 font-sans text-[0.625rem] font-medium [&_svg:not([class*='size-'])]:size-3 pointer-events-none inline-flex items-center justify-center select-none",
        className,
      )}
      {...props}
    />
  );
}

function KbdGroup({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <kbd
      data-slot="kbd-group"
      className={cn("gap-1 inline-flex items-center", className)}
      {...props}
    />
  );
}

export { Kbd, KbdGroup };
