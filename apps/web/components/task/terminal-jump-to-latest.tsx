"use client";

import { useEffect, useState } from "react";
import type { Terminal } from "@xterm/xterm";
import { IconArrowDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

function isAboveBottom(terminal: Terminal): boolean {
  const buffer = terminal.buffer.active;
  return buffer.type === "normal" && buffer.viewportY < buffer.baseY;
}

export function TerminalJumpToLatest({
  terminal,
  enabled,
}: {
  terminal: Terminal | null;
  enabled: boolean;
}) {
  const { t } = useTranslation();
  const [, setAboveBottom] = useState(false);

  useEffect(() => {
    if (!terminal || !enabled) return;
    const update = () => setAboveBottom(isAboveBottom(terminal));
    update();
    const subscriptions = [
      terminal.onScroll(update),
      terminal.onWriteParsed(update),
      terminal.onResize(update),
      terminal.buffer.onBufferChange(update),
    ];
    return () => subscriptions.forEach((subscription) => subscription.dispose());
  }, [terminal, enabled]);

  if (!terminal || !enabled || !isAboveBottom(terminal)) return null;
  return (
    <Button
      type="button"
      variant="secondary"
      size="icon"
      className="absolute bottom-3 right-6 z-10 size-7 shadow-md max-md:size-11 [@media(pointer:coarse)]:size-11"
      aria-label={t("task:jumpToLatestOutput")}
      title={t("task:jumpToLatestOutput")}
      onClick={() => terminal.scrollToBottom()}
    >
      <IconArrowDown className="size-4" />
    </Button>
  );
}
