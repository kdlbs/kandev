"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import { useToast } from "@/components/toast-provider";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import { QueueAdmissionError, QueueFullError, queueMessage } from "@/lib/api/domains/queue-api";
import { generateUUID } from "@/lib/utils";

export const MAX_SEND_BACK_CODE_POINTS = 4000;

const ACCEPTING_STATES = new Set(["STARTING", "RUNNING", "IDLE", "WAITING_FOR_INPUT", "COMPLETED"]);

/** Whether a session in `state` accepts a queued message. */
export function sessionAcceptsMessage(state: string | undefined): boolean {
  return state !== undefined && ACCEPTING_STATES.has(state);
}

type SessionRead =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; incarnationId: string | undefined; state: string | undefined };

type TFn = ReturnType<typeof useTranslation>["t"];

function sendErrorText(error: unknown, t: TFn): string {
  if (error instanceof QueueFullError) return t("coordinator:sendBackQueueFull");
  if (error instanceof QueueAdmissionError) {
    const copy: Record<QueueAdmissionError["code"], string> = {
      validation: t("task:queueAdmissionValidation"),
      "identity-conflict": t("task:queueAdmissionIdentityConflict"),
      "session-unavailable": t("task:queueAdmissionSessionUnavailable"),
      unavailable: t("task:queueAdmissionUnavailable"),
    };
    return copy[error.code];
  }
  return t("coordinator:sendBackFailed");
}

function useSessionRead(sessionId: string) {
  const [read, setRead] = useState<SessionRead>({ status: "loading" });
  const seqRef = useRef(0);
  const load = useCallback(() => {
    const seq = ++seqRef.current;
    setRead({ status: "loading" });
    fetchTaskSession(sessionId, { cache: "no-store" })
      .then((response) => {
        if (seq !== seqRef.current) return;
        setRead({
          status: "ready",
          incarnationId: response.session.queue_incarnation_id,
          state: response.session.state,
        });
      })
      .catch(() => {
        if (seq === seqRef.current) setRead({ status: "error" });
      });
  }, [sessionId]);
  useEffect(() => {
    load();
    return () => {
      seqRef.current += 1;
    };
  }, [load]);
  return { read, retry: load };
}

type SendBackFormProps = {
  taskId: string;
  sessionId: string;
  /** The task identifier, or the task id when unknown. */
  label: string;
  onClose: () => void;
};

/**
 * The inline note form of a Ready to merge row: a queued message to the
 * task's primary session, sent with the user's own authority. The queue id
 * belongs to this form instance and is reused only while the note is the one
 * last attempted.
 */
// eslint-disable-next-line max-lines-per-function -- one form owns the read, the draft and the send lock
export function SendBackForm({ taskId, sessionId, label, onClose }: SendBackFormProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { read, retry } = useSessionRead(sessionId);
  const [note, setNote] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const sendingRef = useRef(false);
  const mountedRef = useRef(true);
  const attemptRef = useRef<{ id: string; note: string } | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    areaRef.current?.focus();
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const trimmed = note.trim();
  const used = Array.from(trimmed).length;
  const accepting = read.status === "ready" && sessionAcceptsMessage(read.state);
  const incarnationId = read.status === "ready" ? read.incarnationId : undefined;
  const refused = read.status === "ready" && (!accepting || !incarnationId);
  const canSend =
    !sending &&
    read.status === "ready" &&
    !refused &&
    used > 0 &&
    used <= MAX_SEND_BACK_CODE_POINTS;

  async function send(e: FormEvent) {
    e.preventDefault();
    if (sendingRef.current || !canSend || !incarnationId) return;
    sendingRef.current = true;
    setSending(true);
    setError(null);
    const attempt =
      attemptRef.current?.note === trimmed
        ? attemptRef.current
        : { id: generateUUID(), note: trimmed };
    attemptRef.current = attempt;
    try {
      await queueMessage({
        session_id: sessionId,
        session_incarnation_id: incarnationId,
        task_id: taskId,
        content: trimmed,
        client_queue_id: attempt.id,
      });
      attemptRef.current = null;
      toast({ title: t("coordinator:sendBackSent", { card: label }), variant: "success" });
      if (mountedRef.current) onClose();
    } catch (err) {
      if (mountedRef.current) setError(sendErrorText(err, t));
    } finally {
      sendingRef.current = false;
      if (mountedRef.current) setSending(false);
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      onClose();
    }
  }

  return (
    <form className="space-y-2 p-2" onSubmit={(e) => void send(e)} onKeyDown={handleKeyDown}>
      <label htmlFor={`send-back-${taskId}`} className="text-xs font-medium">
        {t("coordinator:sendBackLabel", { card: label })}
      </label>
      <Textarea
        id={`send-back-${taskId}`}
        ref={areaRef}
        value={note}
        disabled={sending}
        onChange={(e) => setNote(e.target.value)}
      />
      <p className="text-muted-foreground text-xs/relaxed">
        {t("coordinator:sendBackCounter", { used, max: MAX_SEND_BACK_CODE_POINTS })}
      </p>
      {read.status === "error" && (
        <p role="alert" className="text-destructive flex items-center gap-2 text-xs/relaxed">
          {t("coordinator:sendBackCheckFailed")}
          <Button type="button" variant="ghost" size="sm" onClick={retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </p>
      )}
      {refused && (
        <p role="alert" className="text-destructive text-xs/relaxed">
          {t("coordinator:sendBackNotAccepting")}
        </p>
      )}
      {error && (
        <p role="alert" className="text-destructive text-xs/relaxed">
          {error}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={!canSend} className="min-h-11 sm:min-h-0">
          {sending && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:sendBackSend")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={sending}
          onClick={onClose}
          className="min-h-11 sm:min-h-0"
        >
          {t("coordinator:cancel")}
        </Button>
      </div>
    </form>
  );
}
