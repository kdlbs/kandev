import { useState, useCallback } from "react";

/**
 * Fallback copy method for non-secure contexts (HTTP)
 * where navigator.clipboard is not available
 */
function fallbackCopy(text: string): boolean {
  const textArea = document.createElement("textarea");
  textArea.value = text;
  // Avoid scrolling to bottom
  textArea.style.top = "0";
  textArea.style.left = "0";
  textArea.style.position = "fixed";
  textArea.style.opacity = "0";
  document.body.appendChild(textArea);
  textArea.focus();
  textArea.select();

  let success = false;
  try {
    success = document.execCommand("copy");
  } catch {
    success = false;
  }

  document.body.removeChild(textArea);
  return success;
}

export function useCopyToClipboard(duration = 2000) {
  const [copied, setCopied] = useState(false);

  const copy = useCallback(
    async (text: string) => {
      let success = false;

      // Try modern clipboard API first
      if (navigator.clipboard?.writeText) {
        try {
          await navigator.clipboard.writeText(text);
          success = true;
        } catch {
          // Fall through to fallback
        }
      }

      // Fallback for non-secure contexts
      if (!success) {
        success = fallbackCopy(text);
      }

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
