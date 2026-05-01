import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";

import { cn } from "./lib/utils";

const badgeVariants = cva(
  "h-5 gap-1 rounded-md border border-transparent px-2 py-0.5 text-[0.625rem] font-medium transition-colors has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&>svg]:size-2.5! inline-flex items-center justify-center w-fit whitespace-nowrap shrink-0 [&>svg]:pointer-events-none focus-visible:border-ring focus-visible:ring-ring/35 focus-visible:ring-[2px] aria-invalid:border-destructive aria-invalid:ring-destructive/25 aria-invalid:ring-[2px] data-[disabled=true]:opacity-45 overflow-hidden group/badge",
  {
    variants: {
      variant: {
        default: "border-primary/20 bg-primary/10 text-primary [a]:hover:bg-primary/15",
        secondary: "border-border bg-secondary text-secondary-foreground [a]:hover:bg-secondary/80",
        destructive:
          "border-destructive/25 bg-destructive/10 text-destructive [a]:hover:bg-destructive/15 focus-visible:ring-destructive/25",
        outline:
          "border-border bg-background text-foreground [a]:hover:bg-accent [a]:hover:text-accent-foreground",
        ghost: "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

function Badge({
  className,
  variant = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"span"> & VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span";

  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  );
}

export { Badge, badgeVariants };
