import * as React from "react";

import { cn } from "./lib/utils";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "border-input bg-background hover:bg-secondary/50 focus-visible:border-ring focus-visible:ring-ring/35 aria-invalid:border-destructive aria-invalid:ring-destructive/25 h-7 rounded-md border px-2 py-0.5 text-sm transition-colors file:h-6 file:text-xs/relaxed file:font-medium focus-visible:ring-[2px] aria-invalid:ring-[2px] md:text-xs/relaxed file:text-foreground placeholder:text-muted-foreground w-full min-w-0 outline-none file:inline-flex file:border-0 file:bg-transparent disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-muted disabled:opacity-55",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
