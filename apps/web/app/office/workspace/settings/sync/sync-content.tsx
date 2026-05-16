"use client";

import { IconArrowDown, IconArrowUp, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { SyncDiffPane } from "./sync-diff-pane";
import { useSyncState } from "./use-sync-state";

export function SyncContent() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const sync = useSyncState(activeWorkspaceId);

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-6 py-3 border-b border-border shrink-0">
        <div>
          <h1 className="text-base font-medium">Sync</h1>
          <p className="text-xs text-muted-foreground mt-0.5">
            Compare workspace database with on-disk configuration files.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={sync.refresh}
          disabled={sync.loading}
          className="cursor-pointer"
        >
          <IconRefresh className="h-4 w-4 mr-1.5" />
          Refresh
        </Button>
      </div>
      <div className="flex-1 min-h-0 overflow-auto">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 p-6">
          <SyncDiffPane
            title="Incoming (Filesystem → Database)"
            description="Apply on-disk YAML files to the database."
            icon={<IconArrowDown className="h-4 w-4" />}
            diff={sync.incoming}
            loading={sync.loading}
            applying={sync.applyingIn}
            applyLabel="Import from filesystem"
            onApply={sync.applyIncoming}
          />
          <SyncDiffPane
            title="Outgoing (Database → Filesystem)"
            description="Write the database state to on-disk YAML files."
            icon={<IconArrowUp className="h-4 w-4" />}
            diff={sync.outgoing}
            loading={sync.loading}
            applying={sync.applyingOut}
            applyLabel="Export to filesystem"
            onApply={sync.applyOutgoing}
          />
        </div>
      </div>
    </div>
  );
}
