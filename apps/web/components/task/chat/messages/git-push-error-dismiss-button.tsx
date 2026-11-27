"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { toast } from "@/lib/toast/sonner";
import { getWebSocketClient } from "@/lib/ws/connection";
import { isGitPushErrorMessage } from "@/lib/utils/git-push-error-message";
import type { Message } from "@/lib/types/http";

export function GitPushErrorDismissAction({ message }: { message: Message }) {
  if (!isGitPushErrorMessage(message)) return null;
  return <GitPushErrorDismissButton message={message} />;
}

type DismissGitPushErrorResponse = {
  message_id: string;
  dismissed_at: string;
};

export function GitPushErrorDismissButton({ message }: { message: Message }) {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const [pending, setPending] = useState(false);

  const dismiss = async () => {
    if (pending) return;
    setPending(true);
    try {
      const client = getWebSocketClient();
      if (!client) {
        toast.error(t("common:requestFailed"));
        return;
      }
      const response = await client.request<DismissGitPushErrorResponse>(
        "message.dismiss_git_push_error",
        { message_id: message.id },
      );
      if (response.message_id !== message.id || !response.dismissed_at) {
        throw new Error();
      }
      const current = store
        .getState()
        .messages.bySession[message.session_id]?.find((entry) => entry.id === message.id);
      if (current) {
        store.getState().updateMessage({
          ...current,
          metadata: {
            ...current.metadata,
            git_operation_error_dismissed_at: response.dismissed_at,
          },
        });
      }
    } catch {
      toast.error(t("common:requestFailed"));
    } finally {
      setPending(false);
    }
  };

  return (
    <Button
      variant="outline"
      size="default"
      className={`w-full gap-1.5 text-xs cursor-pointer sm:w-auto ${controlSizingClassName("standard", "max-md:min-h-11 [@media(pointer:coarse)]:min-h-11")}`}
      disabled={pending}
      onClick={dismiss}
      data-testid="git-push-error-dismiss-button"
    >
      {t("task:dismiss")}
    </Button>
  );
}
