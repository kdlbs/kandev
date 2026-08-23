import * as React from "react";
import { cn } from "./lib/utils";

/**
 * Keeps a persistent rotation on an HTML transform target so its SVG child can
 * remain static and be promoted without per-frame SVG style work.
 */
function CompositorSpin({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span {...props} className={cn("inline-flex animate-spin will-change-transform", className)} />
  );
}

export { CompositorSpin };
