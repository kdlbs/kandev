import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowsMaximize, IconRefresh, IconSparkles, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { QuickChatSession } from "@/lib/state/slices/ui/types";
import { ConfigChatRestartConfirmation } from "./config-chat-restart-confirmation";

type Props = {
  session?: QuickChatSession;
  busy: boolean;
  onRestart: (session: QuickChatSession) => void;
  onExpand: () => void;
  onClose: () => void;
};

const CONTROL_CLASS =
  "size-7 min-h-7 min-w-7 max-md:size-11 max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:size-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11 cursor-pointer";

export function ConfigChatHeader({ session, busy, onRestart, onExpand, onClose }: Props) {
  const { t } = useTranslation();
  const restartRef = useRef<HTMLButtonElement>(null);
  const [confirmedSession, setConfirmedSession] = useState<QuickChatSession | null>(null);
  const restartDisabled = busy || !session?.taskId;
  const confirm = () => {
    if (confirmedSession && confirmedSession.sessionId === session?.sessionId && !busy)
      onRestart(confirmedSession);
    setConfirmedSession(null);
  };
  return (
    <header className="flex h-12 shrink-0 items-center justify-between gap-2 border-b bg-muted/30 px-3 pr-1">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <IconSparkles className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate text-sm font-medium">{t("common:configurationChat")}</span>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex" tabIndex={restartDisabled ? 0 : -1}>
              <Button
                ref={restartRef}
                type="button"
                size="icon"
                variant="ghost"
                className={CONTROL_CLASS}
                disabled={restartDisabled}
                onClick={() => setConfirmedSession(session ?? null)}
                aria-label={t("configChat:restartSession")}
              >
                <IconRefresh className="size-4" aria-hidden="true" />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{t("configChat:restartSession")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex" tabIndex={busy ? 0 : -1}>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className={CONTROL_CLASS}
                disabled={busy}
                onClick={onExpand}
                aria-label={t("configChat:openInQuickChat")}
              >
                <IconArrowsMaximize className="size-4" aria-hidden="true" />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{t("configChat:openInQuickChat")}</TooltipContent>
        </Tooltip>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className={CONTROL_CLASS}
          onClick={onClose}
          aria-label={t("configChat:closePanel")}
        >
          <IconX className="size-4" aria-hidden="true" />
        </Button>
      </div>
      <ConfigChatRestartConfirmation
        open={!!confirmedSession}
        targetKey={session?.sessionId ?? ""}
        anchorRef={restartRef}
        onOpenChange={(open) => {
          if (!open) setConfirmedSession(null);
        }}
        onConfirm={confirm}
      />
    </header>
  );
}
