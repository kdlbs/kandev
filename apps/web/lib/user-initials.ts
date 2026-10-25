/**
 * Initials for a person's display name, for avatar fallbacks.
 *
 * First and last word, so "Ana Maria Ferreira" reads as "AF" rather than "AM":
 * a middle name should not displace the family name people recognise.
 *
 * Note the deliberately different `getInitials` in commit-detail-panel.tsx,
 * which takes the first two words of a git author string. That is a separate
 * rule for separate input, not a copy of this one.
 */
export function initialsFor(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return `${parts[0]![0]}${parts[parts.length - 1]![0]}`.toUpperCase();
}
