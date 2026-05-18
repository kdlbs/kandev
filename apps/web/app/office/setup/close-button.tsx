"use client";

import { useRouter } from "next/navigation";
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
      className="absolute -top-12 -left-12 inline-flex h-9 w-9 items-center justify-center rounded-full border border-border text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer"
    >
      <IconX className="h-4 w-4" />
    </button>
  );
}
