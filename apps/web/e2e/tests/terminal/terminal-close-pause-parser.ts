function isTerminalDestroyFrame(part: string): boolean {
  if (!part.trim()) return false;
  try {
    const frame = JSON.parse(part) as { type?: unknown; action?: unknown };
    return frame.type === "request" && frame.action === "user_shell.destroy";
  } catch {
    return false;
  }
}

export function partitionTerminalDestroyRequest(
  message: string | Buffer,
): { destroyFrame: string; passthrough: string | null } | null {
  if (typeof message !== "string") return null;
  const frames = message.split("\n");
  const destroyIndex = frames.findIndex(isTerminalDestroyFrame);
  if (destroyIndex === -1) return null;
  const passthrough = frames.filter((_, index) => index !== destroyIndex).join("\n");
  return {
    destroyFrame: frames[destroyIndex]!,
    passthrough: passthrough.trim() ? passthrough : null,
  };
}
