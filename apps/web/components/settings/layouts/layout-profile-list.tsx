"use client";

import { IconCopy, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import {
  BUILT_IN_LAYOUT_PROFILES,
  getLayoutProfileCompatibility,
  resolveEffectiveDefaultLayout,
  type BuiltInLayoutProfileId,
} from "@/lib/layout/layout-profiles";
import type { SavedLayout } from "@/lib/types/http";

export type LayoutProfileSelection =
  | { kind: "built-in"; id: BuiltInLayoutProfileId }
  | { kind: "custom"; id: string };

type LayoutProfileListProps = {
  profiles: SavedLayout[];
  selection: LayoutProfileSelection;
  onSelect: (selection: LayoutProfileSelection) => void;
  onCreate: () => void;
  onDuplicate: () => void;
};

function isSelected(
  selection: LayoutProfileSelection,
  kind: LayoutProfileSelection["kind"],
  id: string,
) {
  return selection.kind === kind && selection.id === id;
}

const profileButtonClass =
  "flex min-h-11 w-full cursor-pointer items-start justify-between gap-2 rounded-md border px-3 py-2 text-left transition-colors";

export function LayoutProfileList({
  profiles,
  selection,
  onSelect,
  onCreate,
  onDuplicate,
}: LayoutProfileListProps) {
  const effectiveDefault = resolveEffectiveDefaultLayout(profiles);
  return (
    <aside className="min-w-0 space-y-3" aria-label="Layout profiles">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          className="min-h-11 cursor-pointer sm:min-h-8"
          onClick={onCreate}
          data-testid="layout-profile-create"
        >
          <IconPlus className="mr-1.5 h-4 w-4" /> New
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          onClick={onDuplicate}
          data-testid="layout-profile-duplicate"
        >
          <IconCopy className="mr-1.5 h-4 w-4" /> Duplicate
        </Button>
      </div>

      <div className="space-y-1.5">
        <h4 className="text-xs font-medium uppercase text-muted-foreground">Templates</h4>
        {BUILT_IN_LAYOUT_PROFILES.map((profile) => (
          <button
            key={profile.id}
            type="button"
            className={cn(
              profileButtonClass,
              isSelected(selection, "built-in", profile.id)
                ? "border-primary bg-primary/5"
                : "hover:bg-muted/50",
            )}
            onClick={() => onSelect({ kind: "built-in", id: profile.id })}
            data-testid={`layout-profile-built-in-${profile.id}`}
          >
            <span className="min-w-0">
              <span className="block text-sm font-medium">{profile.name}</span>
              <span className="block text-xs text-muted-foreground">{profile.description}</span>
            </span>
            {profile.id === "default" && effectiveDefault.source === "built-in" && (
              <Badge variant="secondary">Default</Badge>
            )}
          </button>
        ))}
      </div>

      <div className="space-y-1.5">
        <h4 className="text-xs font-medium uppercase text-muted-foreground">Custom</h4>
        {profiles.length === 0 && (
          <p className="py-3 text-sm text-muted-foreground">No custom profiles</p>
        )}
        {profiles.map((profile) => {
          const compatibility = getLayoutProfileCompatibility(profile);
          return (
            <button
              key={profile.id}
              type="button"
              className={cn(
                profileButtonClass,
                isSelected(selection, "custom", profile.id)
                  ? "border-primary bg-primary/5"
                  : "hover:bg-muted/50",
              )}
              onClick={() => onSelect({ kind: "custom", id: profile.id })}
              data-testid={`layout-profile-custom-${profile.id}`}
            >
              <span className="min-w-0 truncate text-sm font-medium">{profile.name}</span>
              <span className="flex shrink-0 gap-1">
                {effectiveDefault.source === "custom" &&
                  effectiveDefault.profile.id === profile.id && (
                    <Badge variant="secondary">Default</Badge>
                  )}
                {compatibility.status === "legacy" && <Badge variant="outline">Unavailable</Badge>}
              </span>
            </button>
          );
        })}
      </div>
    </aside>
  );
}
