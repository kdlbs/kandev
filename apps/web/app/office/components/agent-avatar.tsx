"use client";

import { cn } from "@/lib/utils";

/**
 * Agent avatar: 1–2 initials from the agent name in a per-name tinted
 * square. The tint is picked from a small palette via a stable hash of
 * the name, so each agent looks distinct but the same agent always
 * renders the same color.
 *
 * `role` is accepted for forward-compat (callers already pass it) but
 * is intentionally unused right now — every agent uses the same
 * initials treatment.
 */

type AgentAvatarProps = {
  role?: string | null;
  name?: string | null;
  className?: string;
  size?: "sm" | "md" | "lg";
};

const SIZE: Record<NonNullable<AgentAvatarProps["size"]>, string> = {
  sm: "h-6 w-6 text-[10px]",
  md: "h-8 w-8 text-xs",
  lg: "h-10 w-10 text-sm",
};

const TINTS = [
  "bg-amber-500/15 text-amber-600 ring-amber-500/30 dark:text-amber-400",
  "bg-emerald-500/15 text-emerald-600 ring-emerald-500/30 dark:text-emerald-400",
  "bg-violet-500/15 text-violet-600 ring-violet-500/30 dark:text-violet-400",
  "bg-blue-500/15 text-blue-600 ring-blue-500/30 dark:text-blue-400",
  "bg-rose-500/15 text-rose-600 ring-rose-500/30 dark:text-rose-400",
  "bg-cyan-500/15 text-cyan-600 ring-cyan-500/30 dark:text-cyan-400",
  "bg-orange-500/15 text-orange-600 ring-orange-500/30 dark:text-orange-400",
];

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

function tintFor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) | 0;
  return TINTS[Math.abs(h) % TINTS.length];
}

/**
 * Return the per-agent tint as a Tailwind class string. Use the same
 * stable hash as the avatar so a chip and an avatar for the same agent
 * always share a color. Callers compose the result into their own
 * elements (tabs, dots, badges) when a full avatar is overkill.
 */
export function agentTint(name: string | null | undefined): string {
  return tintFor((name ?? "").trim() || "Agent");
}

export function AgentAvatar({ name, className, size = "md" }: AgentAvatarProps) {
  const safeName = (name ?? "").trim() || "Agent";
  const tint = tintFor(safeName);
  return (
    <span
      className={cn(
        "inline-flex items-center justify-center rounded-md ring-1 shrink-0 font-semibold tracking-tight",
        SIZE[size],
        tint,
        className,
      )}
      aria-hidden
    >
      {initials(safeName)}
    </span>
  );
}
