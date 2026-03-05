"use client";

import { useState, useCallback, memo } from "react";
import { IconGitCommit, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message } from "@/lib/types/http";
import type { GitOperationErrorMetadata } from "@/components/task/chat/types";

export const GitOperationErrorMessage = memo(function GitOperationErrorMessage({
  comment,
}: {
  comment: Message;
}) {
  const [fixSent, setFixSent] = useState(false);
  const metadata = comment.metadata as GitOperationErrorMetadata | undefined;

  const handleFix = useCallback(async () => {
    const client = getWebSocketClient();
    if (!client || !metadata) return;

    setFixSent(true);
    const prompt = [
      `The git ${metadata.operation} command failed with the following error:`,
      "",
      "```",
      metadata.error_output,
      "```",
      "",
      "Please fix the issues reported above.",
    ].join("\n");

    try {
      await client.request("message.add", {
        task_id: metadata.task_id,
        session_id: metadata.session_id,
        content: prompt,
      });
    } catch {
      setFixSent(false);
    }
  }, [metadata]);

  const operation = metadata?.operation || "operation";
  const title = comment.content || `Git ${operation} failed`;

  return (
    <div className="w-full" data-testid="git-operation-error-message">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconGitCommit className="h-4 w-4 text-red-500" />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="text-xs font-medium text-red-600 dark:text-red-400">{title}</div>

          {metadata?.error_output && (
            <pre className="mt-1.5 text-[11px] font-mono text-muted-foreground bg-muted/50 rounded p-2 overflow-auto max-h-[300px] whitespace-pre-wrap break-words">
              {metadata.error_output}
            </pre>
          )}

          <div className="mt-2">
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs cursor-pointer gap-1.5"
              disabled={fixSent}
              onClick={handleFix}
              data-testid="git-fix-button"
            >
              <IconSparkles className="h-3 w-3" />
              {fixSent ? "Fix requested" : "Fix"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
});
