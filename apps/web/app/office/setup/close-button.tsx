"use client";

import { useRouter } from "@/lib/routing/client-router";
import { IconX } from "@tabler/icons-react";

type CloseButtonProps = {
  /** Where to navigate when the user dismisses the wizard. */
  href: string;
};

export function CloseButton({ href }: CloseButtonProps) {
  const router = useRouter();
  return (
    <button
      type="button"
      onClick={() => router.push(href)}
      aria-label="Cancel"
      className="fixed top-4 right-4 z-10 inline-flex h-9 w-9 items-center justify-center rounded-full border border-border bg-background text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer lg:absolute lg:bg-transparent lg:-top-12 lg:-left-12 lg:right-auto"
    >
      <IconX className="h-4 w-4" />
    </button>
  );
}
