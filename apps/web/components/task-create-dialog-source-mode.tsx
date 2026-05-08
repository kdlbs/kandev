"use client";

import { cn } from "@/lib/utils";

export type SourceMode = "workspace" | "url" | "scratch";

function resolveMode(useGitHubUrl: boolean, noRepository: boolean): SourceMode {
  if (noRepository) return "scratch";
  if (useGitHubUrl) return "url";
  return "workspace";
}

type SourceModeSwitchProps = {
  useGitHubUrl: boolean;
  noRepository: boolean;
  onToggleGitHubUrl?: () => void;
  onToggleNoRepository?: () => void;
};

/**
 * Three-mode segmented control rendered at the right edge of the chip row:
 *   - Repo  : pick a workspace repository (default)
 *   - URL   : paste a GitHub URL
 *   - None  : run in a scratch workspace or a folder you pick
 *
 * Switching modes calls the underlying single-mode togglers in the right
 * order so we never end up in two modes at once. Hidden when no togglers
 * are provided (e.g. locked-fields wrapper).
 */
export function SourceModeSwitch({
  useGitHubUrl,
  noRepository,
  onToggleGitHubUrl,
  onToggleNoRepository,
}: SourceModeSwitchProps) {
  if (!onToggleGitHubUrl && !onToggleNoRepository) return null;
  const mode = resolveMode(useGitHubUrl, noRepository);
  const setMode = (next: SourceMode) =>
    switchMode({
      from: mode,
      to: next,
      onToggleGitHubUrl,
      onToggleNoRepository,
    });
  return (
    <div
      role="radiogroup"
      aria-label="Source"
      className="ml-auto inline-flex items-center rounded-md border border-border/60 bg-muted/20 p-0.5"
    >
      <ModeButton label="Repo" mode="workspace" active={mode} onSelect={setMode} />
      {onToggleGitHubUrl && <ModeButton label="URL" mode="url" active={mode} onSelect={setMode} />}
      {onToggleNoRepository && (
        <ModeButton label="None" mode="scratch" active={mode} onSelect={setMode} />
      )}
    </div>
  );
}

function switchMode({
  from,
  to,
  onToggleGitHubUrl,
  onToggleNoRepository,
}: {
  from: SourceMode;
  to: SourceMode;
  onToggleGitHubUrl?: () => void;
  onToggleNoRepository?: () => void;
}) {
  if (from === to) return;
  if (to === "url") {
    if (from === "scratch") onToggleNoRepository?.();
    onToggleGitHubUrl?.();
    return;
  }
  if (to === "scratch") {
    if (from === "url") onToggleGitHubUrl?.();
    onToggleNoRepository?.();
    return;
  }
  // workspace
  if (from === "url") onToggleGitHubUrl?.();
  else if (from === "scratch") onToggleNoRepository?.();
}

function ModeButton({
  label,
  mode,
  active,
  onSelect,
}: {
  label: string;
  mode: SourceMode;
  active: SourceMode;
  onSelect: (m: SourceMode) => void;
}) {
  const isActive = active === mode;
  return (
    <button
      type="button"
      role="radio"
      aria-checked={isActive}
      data-testid={`source-mode-${mode}`}
      onClick={() => onSelect(mode)}
      className={cn(
        "rounded-sm px-2 py-0.5 text-[11px] font-medium transition-colors cursor-pointer",
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {label}
    </button>
  );
}
