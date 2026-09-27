/** Client-side defense for legacy and request-local recovery diagnostics. */
export function sanitizeSessionErrorDetails(error: unknown, limit = 4096): string {
  const source = error instanceof Error ? error.message : error;
  if (typeof source !== "string") return "";
  let text = source
    .replace(/\u001b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, "")
    .replace(/-----BEGIN [^-]*PRIVATE KEY-----[\s\S]*/g, "***")
    .replace(/\b[a-z][a-z0-9+.-]*:\/\/[^\s]+/gi, (value) => {
      try {
        const url = new URL(value);
        return /^(https?|wss?):$/.test(url.protocol) ? `${url.protocol}//${url.hostname}` : "***";
      } catch {
        return "***";
      }
    });
  text = redactAssignments(text)
    .replace(/\b(?:Bearer|Basic)\s+[^\s,;]+/gi, "***")
    .replace(/\b(?:sk-|ghp_|github_pat_|kandev_pat_)[A-Za-z0-9_-]+/g, "***")
    .replace(/\b(?:wrk|ses|run)_[A-Za-z0-9_-]+\b/g, "***")
    .replace(/\b[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\b/gi, "***")
    .replace(/\b[A-Za-z]:\\[^\s<>"']+/g, "***")
    .replace(/(?<![:/\w])\/(?!\/)[^\s<>"']+/g, "***")
    .replace(/[A-Za-z0-9+/=_-]{32,}/g, "***");
  return text
    .slice(0, limit)
    .replace(/[\uD800-\uDBFF]$/u, "")
    .trim();
}

function redactAssignments(text: string): string {
  const lines = text.split("\n");
  const safe: string[] = [];
  for (const line of lines) {
    const match =
      /(?:["']?[\w.-]*(?:password|secret|credential|token|api[_-]?key)[\w.-]*["']?\s*[:=]\s*|Authorization\s*:\s*)/i.exec(
        line,
      );
    if (match) {
      const value = line.slice(match.index + match[0].length);
      safe.push(line.slice(0, match.index) + "***");
      // A malformed or multiline value has no trustworthy diagnostic boundary.
      if (
        !value.trim() ||
        /^[{[]/.test(value) ||
        ((value[0] === '"' || value[0] === "'") && !hasClosingQuote(value))
      )
        break;
      continue;
    }
    safe.push(line.replace(/\bsecret\s+(?!\*\*\*)[^\s]+/gi, "secret ***"));
  }
  return safe.join("\n");
}

function hasClosingQuote(value: string): boolean {
  for (let index = 1; index < value.length; index++) {
    if (value[index] === "\\") index++;
    else if (value[index] === value[0]) return true;
  }
  return false;
}
