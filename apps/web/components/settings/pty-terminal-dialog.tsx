"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import {
  PtyTerminalView,
  type PtySessionStatus,
  type PtyTerminalState,
  type StartPtySession,
} from "./pty-terminal-view";

export type { StartPtySession } from "./pty-terminal-view";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  testIdPrefix?: string;
  startSession: StartPtySession;
  onDone?: () => void;
  initialInput?: string;
  command?: string[];
  presentation?: "standard" | "quick";
};

function SessionStatus({
  status,
  exitCode,
  error,
}: Pick<PtyTerminalState, "status" | "exitCode" | "error">) {
  const { t } = useTranslation();
  if (status === "connecting") {
    return <p className="text-xs text-muted-foreground">{t("agents:startingSession")}</p>;
  }
  if (status === "exited") {
    return (
      <p className="text-xs text-muted-foreground">
        {exitCode != null
          ? t("agents:sessionEndedWithCode", { code: exitCode })
          : t("agents:sessionEnded")}
      </p>
    );
  }
  if (status === "error" && error) {
    return <p className="text-xs text-destructive">{error}</p>;
  }
  return null;
}

function PtySessionView({
  startSession,
  testIdPrefix,
  initialInput,
  onDone,
  presentation,
}: {
  startSession: StartPtySession;
  testIdPrefix?: string;
  initialInput?: string;
  onDone: () => void;
  presentation: "standard" | "quick";
}) {
  const [state, setState] = useState<PtyTerminalState>({
    status: "connecting" as PtySessionStatus,
    sessionId: null,
    exitCode: null,
    error: null,
  });
  const { t } = useTranslation();
  return (
    <>
      <PtyTerminalView
        startSession={startSession}
        testIdPrefix={testIdPrefix}
        initialInput={initialInput}
        className={
          presentation === "quick"
            ? "min-h-0 flex-1 rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
            : "h-[420px] rounded-md bg-[#0b0b0c] p-2 overflow-hidden"
        }
        onStateChange={setState}
      />
      <SessionStatus {...state} />
      <DialogFooter className={presentation === "quick" ? "shrink-0" : undefined}>
        <Button
          type="button"
          onClick={onDone}
          className="cursor-pointer"
          data-testid={`${testIdPrefix ?? "pty"}-done`}
        >
          {t("agents:done")}
        </Button>
      </DialogFooter>
    </>
  );
}

/** Dialog wrapper used by agent-login and host-shell settings flows. */
export function PtyTerminalDialog({
  open,
  onOpenChange,
  title,
  description,
  testIdPrefix,
  startSession,
  onDone,
  initialInput,
  command,
  presentation = "standard",
}: Props) {
  const handleDone = () => {
    onDone?.();
    onOpenChange(false);
  };
  const cmdLine = command && command.length > 0 ? command.join(" ") : null;
  const dialogClassName =
    presentation === "quick"
      ? "!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex flex-col gap-3 overflow-hidden p-4 [padding-top:max(1rem,env(safe-area-inset-top))] [padding-bottom:max(1rem,env(safe-area-inset-bottom))] sm:!left-1/2 sm:!top-1/2 sm:!h-[85dvh] sm:!max-h-[85dvh] sm:!w-[min(1100px,calc(100vw-2rem))] sm:!max-w-none sm:!-translate-x-1/2 sm:!-translate-y-1/2"
      : "sm:max-w-[820px]";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={dialogClassName}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {cmdLine && (
          <div
            data-testid={`${testIdPrefix ?? "pty"}-command`}
            className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs"
          >
            <span className="text-muted-foreground">$</span>
            <code className="flex-1 truncate" title={cmdLine}>
              {cmdLine}
            </code>
          </div>
        )}
        {open && (
          <PtySessionView
            startSession={startSession}
            testIdPrefix={testIdPrefix}
            initialInput={initialInput}
            onDone={handleDone}
            presentation={presentation}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
