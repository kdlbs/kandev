"use client";

import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type ChatInputFocusHintProps = {
  visible: boolean;
  className?: string;
};

export function ChatInputFocusHint({ visible, className }: ChatInputFocusHintProps) {
  const { t } = useTranslation();
  if (!visible) return null;

  return (
    <div
      className={cn(
        "absolute top-2 right-2 z-10",
        "flex items-center gap-1.5",
        "px-2 py-1",
        "text-xs text-muted-foreground/60",
        "pointer-events-none",
        "transition-opacity duration-200",
        className,
      )}
    >
      <kbd className="px-1 py-0.5 text-xs font-medium text-muted-foreground/70 border border-border/40 rounded">
        /
      </kbd>
      <span>{t("task:toFocus")}</span>
    </div>
  );
}
