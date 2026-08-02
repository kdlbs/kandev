import { useState, useCallback } from "react";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

export function useCopyToClipboard(duration = 2000) {
  const [copied, setCopied] = useState(false);

  const copy = useCallback(
    async (text: string) => {
      const success = await copyToClipboard(text);

      if (success) {
        setCopied(true);
        setTimeout(() => setCopied(false), duration);
      } else {
        console.error("Failed to copy to clipboard");
      }
    },
    [duration],
  );

  return { copied, copy };
}
