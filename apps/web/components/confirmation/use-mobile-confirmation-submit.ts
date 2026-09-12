"use client";

import { useLayoutEffect, useRef, useState } from "react";

export type ConfirmationCompletionPolicy = "close-before-dispatch" | "await-with-retry";

/** Only owners with retryable forms opt into waiting; ordinary actions dismiss before dispatch. */
export function useMobileConfirmationSubmit({
  closed,
  disabled,
  close,
  onConfirm,
  completionPolicy,
}: {
  closed: boolean;
  disabled: boolean;
  close: () => void;
  onConfirm: () => void | Promise<void>;
  completionPolicy: ConfirmationCompletionPolicy;
}) {
  const submitted = useRef(false);
  const live = useRef(true);
  const [pending, setPending] = useState(false);
  useLayoutEffect(() => {
    live.current = !closed;
    return () => {
      live.current = false;
    };
  }, [closed]);

  const submit = () => {
    if (submitted.current || closed || disabled) return;
    submitted.current = true;
    if (completionPolicy === "close-before-dispatch") {
      close();
      queueMicrotask(() => {
        void Promise.resolve()
          .then(onConfirm)
          .catch(() => undefined);
      });
      return;
    }
    setPending(true);
    void Promise.resolve()
      .then(onConfirm)
      .then(
        () => {
          if (live.current) close();
        },
        () => {
          if (!live.current) return;
          submitted.current = false;
          setPending(false);
        },
      );
  };
  return { submit, submitted, pending };
}
