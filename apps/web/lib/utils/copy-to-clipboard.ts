/**
 * Copy text using the Clipboard API when available, with a DOM fallback for
 * non-secure contexts and browsers that reject the modern API.
 *
 * The fallback keeps a temporary textarea inside the active dialog when there
 * is one so Radix FocusScope does not steal focus before execCommand runs.
 */
function fallbackCopy(text: string): boolean {
  if (typeof document === "undefined") return false;

  const previousActive = document.activeElement as HTMLElement | null;
  const container =
    previousActive?.closest<HTMLElement>('[data-slot="dialog-content"], [role="dialog"]') ??
    document.body;
  const textArea = document.createElement("textarea");
  textArea.value = text;
  textArea.style.top = "0";
  textArea.style.left = "0";
  textArea.style.position = "fixed";
  textArea.style.opacity = "0";
  container.appendChild(textArea);
  textArea.focus();
  textArea.select();

  let success = false;
  try {
    success = document.execCommand("copy");
  } catch {
    success = false;
  } finally {
    try {
      container.removeChild(textArea);
    } catch {
      // The dialog may have closed while the copy was in progress.
    }
    previousActive?.focus();
  }
  return success;
}

export async function copyToClipboard(text: string): Promise<boolean> {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the DOM fallback.
    }
  }

  return fallbackCopy(text);
}
