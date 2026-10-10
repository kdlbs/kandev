import { useState, useCallback, useEffect, useRef } from "react";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

export function useCopyToClipboard(duration = 2000) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timeoutRef.current !== null) clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    },
    [],
  );

  const copy = useCallback(
    async (text: string) => {
      const success = await copyToClipboard(text);

      if (success) {
        if (timeoutRef.current !== null) clearTimeout(timeoutRef.current);
        setCopied(true);
        timeoutRef.current = setTimeout(() => {
          timeoutRef.current = null;
          setCopied(false);
        }, duration);
      } else {
        console.error("Failed to copy to clipboard");
      }
    },
    [duration],
  );

  return { copied, copy };
}
