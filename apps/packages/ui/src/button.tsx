import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";

import { cn } from "./lib/utils";

const buttonVariants = cva(
  "focus-visible:border-ring focus-visible:ring-ring/35 aria-invalid:border-destructive aria-invalid:ring-destructive/25 rounded-md border border-transparent bg-clip-padding text-xs/relaxed font-medium focus-visible:ring-[2px] aria-invalid:ring-[2px] [&_svg:not([class*='size-'])]:size-4 inline-flex cursor-pointer items-center justify-center whitespace-nowrap transition-colors disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:pointer-events-none shrink-0 [&_svg]:shrink-0 outline-none group/button select-none",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90 active:bg-primary/95",
        outline:
          "border-border bg-background hover:bg-accent hover:text-accent-foreground active:bg-accent/85 aria-expanded:bg-accent aria-expanded:text-accent-foreground",
        secondary:
          "border-border/70 bg-secondary text-secondary-foreground hover:bg-secondary/80 active:bg-secondary/90 aria-expanded:bg-secondary aria-expanded:text-secondary-foreground",
        ghost:
          "hover:bg-accent hover:text-accent-foreground active:bg-accent/85 aria-expanded:bg-accent aria-expanded:text-accent-foreground",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90 active:bg-destructive/95 focus-visible:border-destructive focus-visible:ring-destructive/25",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default:
          "h-7 gap-1 px-2 text-xs/relaxed has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        xs: "h-5 gap-1 rounded-sm px-2 text-[0.625rem] has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-2.5",
        sm: "h-6 gap-1 px-2 text-xs/relaxed has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        lg: "h-8 gap-1 px-2.5 text-xs/relaxed has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2 [&_svg:not([class*='size-'])]:size-4",
        icon: "size-7 [&_svg:not([class*='size-'])]:size-3.5",
        "icon-xs": "size-5 rounded-sm [&_svg:not([class*='size-'])]:size-2.5",
        "icon-sm": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-lg": "size-8 [&_svg:not([class*='size-'])]:size-4",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
  }) {
  const Comp = asChild ? Slot.Root : "button";

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}

export { Button, buttonVariants };
