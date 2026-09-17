"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useRouter } from "@/lib/routing/client-router";
import { fetchJson } from "@/lib/api/client";
import { toast } from "@/lib/toast/sonner";

export function OpenConversationButton({
  agentId,
  workspaceId,
}: {
  agentId: string;
  workspaceId: string;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [opening, setOpening] = useState(false);
  async function openConversation() {
    setOpening(true);
    try {
      const { channel } = await fetchJson<{ channel: { task_id: string } }>(
        `/api/v1/office/workspaces/${encodeURIComponent(workspaceId)}/agents/${encodeURIComponent(agentId)}/conversation`,
        { init: { method: "POST" } },
      );
      router.push(
        `/workspace/conversations/${channel.task_id}?workspaceId=${encodeURIComponent(workspaceId)}`,
      );
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("office:conversationOpenFailed"));
    } finally {
      setOpening(false);
    }
  }
  return (
    <Button onClick={openConversation} disabled={opening} data-testid="open-agent-conversation">
      {t(opening ? "office:conversationOpening" : "office:conversationOpen")}
    </Button>
  );
}
