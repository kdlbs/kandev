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

type ControlledSourceModeOption<T extends string> = {
  value: T;
  label: string;
  testId: string;
  legacyTestId?: string;
};

type ControlledSourceModeSwitchProps<T extends string> = {
  mode: T;
  options: readonly ControlledSourceModeOption<T>[];
  onModeChange: (mode: T) => void;
  touchSized?: boolean;
};

export function ControlledSourceModeSwitch<T extends string>({
  mode,
  options,
  onModeChange,
  touchSized = false,
}: ControlledSourceModeSwitchProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label="Source"
      className="inline-flex items-center rounded-md border border-border/60 bg-muted/20 p-0.5"
    >
      {options.map((option) => {
        const isActive = mode === option.value;
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={isActive}
            data-testid={option.testId}
            data-legacy-testid={option.legacyTestId}
            onClick={() => onModeChange(option.value)}
            className={cn(
              "rounded-sm px-2 text-[11px] font-medium transition-colors cursor-pointer",
              touchSized ? "min-h-11" : "py-0.5",
              isActive
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

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
    <div className="ml-auto">
      <ControlledSourceModeSwitch
        mode={mode}
        onModeChange={setMode}
        options={[
          { value: "workspace", label: "Repo", testId: "source-mode-workspace" },
          ...(onToggleRemote
            ? [
                {
                  value: "remote" as const,
                  label: "Remote",
                  testId: "source-mode-remote",
                  legacyTestId: "toggle-github-url",
                },
              ]
            : []),
          ...(onToggleNoRepository
            ? [{ value: "scratch" as const, label: "None", testId: "source-mode-scratch" }]
            : []),
        ]}
      />
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
