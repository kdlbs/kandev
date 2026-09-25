"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import { getWebSocketClient } from "@/lib/ws/connection";
import { isGitPushErrorMessage } from "@/lib/utils/git-push-error-message";
import type { Message } from "@/lib/types/http";

export function GitPushErrorDismissAction({
  messageId,
  type,
  metadata,
}: {
  messageId: string;
  type: Message["type"];
  metadata: Message["metadata"];
}) {
  if (!isGitPushErrorMessage({ type, metadata })) return null;
  return <GitPushErrorDismissButton messageId={messageId} />;
}

export function GitPushErrorDismissButton({ messageId }: { messageId: string }) {
  const { t } = useTranslation();
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
      await client.request("message.dismiss_git_push_error", { message_id: messageId });
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
