"use client";

import { useEffect, useState } from "react";

import {
  fetchWorkspaceRuntimeFootprint,
  type WorkspaceRuntimeFootprint,
} from "@/lib/api/domains/runtime-footprint-api";

export type WorkspaceRuntimeFootprintLoader = (
  workspaceId: string,
) => Promise<WorkspaceRuntimeFootprint>;

type Reading = {
  workspaceId: string;
  footprint: WorkspaceRuntimeFootprint | null;
  failed: boolean;
};

// One bounded read per mounted workspace, with no timer or shared observer.
export function useWorkspaceRuntimeFootprint(
  workspaceId: string,
  load: WorkspaceRuntimeFootprintLoader = fetchWorkspaceRuntimeFootprint,
) {
  const [reading, setReading] = useState<Reading>({ workspaceId, footprint: null, failed: false });

  useEffect(() => {
    let current = true;
    setReading({ workspaceId, footprint: null, failed: false });
    void load(workspaceId).then(
      (footprint) => {
        if (current) setReading({ workspaceId, footprint, failed: false });
      },
      () => {
        if (current) setReading({ workspaceId, footprint: null, failed: true });
      },
    );
    return () => {
      current = false;
    };
  }, [load, workspaceId]);

  const owned = reading.workspaceId === workspaceId;
  return { footprint: owned ? reading.footprint : null, failed: owned && reading.failed };
}
