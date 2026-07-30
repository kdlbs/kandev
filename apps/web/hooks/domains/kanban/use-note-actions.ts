import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

export function buildEnhanceNoteWithAIContent(currentContent: string): string {
  return [
    "Please enhance and improve the clarity, organization, grammar, and formatting of these task notes using the update_task_note_kandev MCP tool to save the result.",
    "",
    "Current notes:",
    "",
    currentContent.trim() || "(empty)",
  ].join("\n");
}

export function useNoteActions(opts: { resolvedSessionId: string | null; taskId: string | null }) {
  const { toast } = useToast();
  const [isEnhancing, setIsEnhancing] = useState(false);

  const enhanceNoteWithAI = useCallback(
    async (currentContent: string): Promise<boolean> => {
      if (!opts.resolvedSessionId || !opts.taskId || currentContent.trim() === "") return false;
      const client = getWebSocketClient();
      if (!client) {
        toast({
          description: "Not connected — can't enhance note with AI right now",
          variant: "error",
        });
        return false;
      }

      setIsEnhancing(true);
      try {
        await client.request(
          "message.add",
          {
            task_id: opts.taskId,
            session_id: opts.resolvedSessionId,
            content: buildEnhanceNoteWithAIContent(currentContent),
          },
          10000,
        );
        return true;
      } catch (err) {
        console.error("Failed to request note enhancement:", err);
        toast({ description: "Failed to enhance note with AI", variant: "error" });
        return false;
      } finally {
        setIsEnhancing(false);
      }
    },
    [opts.resolvedSessionId, opts.taskId, toast],
  );

  return { enhanceNoteWithAI, isEnhancing };
}
