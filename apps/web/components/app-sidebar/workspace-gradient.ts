/** Stable workspace avatar gradient + initials helpers. Ported from the
 *  former WorkspaceRail so the unified AppSidebar header can keep the same
 *  per-workspace color identity. */

const GRADIENTS = [
  "linear-gradient(135deg, #6366f1, #8b5cf6)",
  "linear-gradient(135deg, #3b82f6, #06b6d4)",
  "linear-gradient(135deg, #10b981, #06b6d4)",
  "linear-gradient(135deg, #f59e0b, #ef4444)",
  "linear-gradient(135deg, #ec4899, #8b5cf6)",
  "linear-gradient(135deg, #14b8a6, #3b82f6)",
  "linear-gradient(135deg, #f97316, #facc15)",
  "linear-gradient(135deg, #84cc16, #10b981)",
];

export function getWorkspaceGradient(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  }
  return GRADIENTS[hash % GRADIENTS.length];
}

export function getWorkspaceInitials(name: string): string {
  const words = (name || "W").trim().split(/\s+/);
  if (words.length >= 2) {
    return (words[0].charAt(0) + words[1].charAt(0)).toUpperCase();
  }
  return words[0].charAt(0).toUpperCase();
}
