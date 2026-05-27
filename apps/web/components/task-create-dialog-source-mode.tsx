"use client";

import { cn } from "@/lib/utils";

export type SourceMode = "workspace" | "remote" | "scratch";

function resolveMode(useRemote: boolean, noRepository: boolean): SourceMode {
  if (noRepository) return "scratch";
  if (useRemote) return "remote";
  return "workspace";
}

type SourceModeSwitchProps = {
  useRemote: boolean;
  noRepository: boolean;
  onToggleRemote?: () => void;
  onToggleNoRepository?: () => void;
};

/**
 * Three-mode segmented control rendered at the right edge of the chip row:
 *   - Repo   : pick a workspace repository (default)
 *   - Remote : paste a GitHub URL (Task 4 renamed "url" → "remote")
 *   - None   : run in a scratch workspace or a folder you pick
 *
 * Switching modes calls the underlying single-mode togglers in the right
 * order so we never end up in two modes at once. Hidden when no togglers
 * are provided (e.g. locked-fields wrapper).
 */
export function SourceModeSwitch({
  useRemote,
  noRepository,
  onToggleRemote,
  onToggleNoRepository,
}: SourceModeSwitchProps) {
  if (!onToggleRemote && !onToggleNoRepository) return null;
  const mode = resolveMode(useRemote, noRepository);
  const setMode = (next: SourceMode) =>
    switchMode({
      from: mode,
      to: next,
      onToggleRemote,
      onToggleNoRepository,
    });
  return (
    <div
      role="radiogroup"
      aria-label="Source"
      className="ml-auto inline-flex items-center rounded-md border border-border/60 bg-muted/20 p-0.5"
    >
      <ModeButton label="Repo" mode="workspace" active={mode} onSelect={setMode} />
      {onToggleRemote && (
        <ModeButton label="Remote" mode="remote" active={mode} onSelect={setMode} />
      )}
      {onToggleNoRepository && (
        <ModeButton label="None" mode="scratch" active={mode} onSelect={setMode} />
      )}
    </div>
  );
}

function switchMode({
  from,
  to,
  onToggleRemote,
  onToggleNoRepository,
}: {
  from: SourceMode;
  to: SourceMode;
  onToggleRemote?: () => void;
  onToggleNoRepository?: () => void;
}) {
  if (from === to) return;
  if (to === "remote") {
    if (from === "scratch") onToggleNoRepository?.();
    onToggleRemote?.();
    return;
  }
  if (to === "scratch") {
    if (from === "remote") onToggleRemote?.();
    onToggleNoRepository?.();
    return;
  }
  // workspace
  if (from === "remote") onToggleRemote?.();
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
  // The Remote button carries `data-legacy-testid="toggle-github-url"` so the older
  // create-task-github-url.spec.ts and subtask.spec.ts can keep selecting it during
  // the migration; the primary `data-testid` follows the source-mode-<mode> convention.
  const legacyTestId = mode === "remote" ? "toggle-github-url" : undefined;
  return (
    <button
      type="button"
      role="radio"
      aria-checked={isActive}
      data-testid={`source-mode-${mode}`}
      data-legacy-testid={legacyTestId}
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
