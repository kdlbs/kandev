"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { IconArrowsLeftRight, IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

export function ConfigSection() {
  const router = useRouter();

  const handleSync = useCallback(() => {
    router.push("/orchestrate/workspace/settings/sync");
  }, [router]);

  const handleExport = useCallback(() => {
    router.push("/orchestrate/workspace/settings/export");
  }, [router]);

  return (
    <section className="space-y-4">
      <h2 className="text-sm font-semibold">Configuration</h2>
      <p className="text-xs text-muted-foreground">
        Sync the workspace database with on-disk YAML files, or download a portable export bundle.
      </p>
      <div className="flex gap-2">
        <Button variant="outline" onClick={handleSync} className="cursor-pointer">
          <IconArrowsLeftRight className="h-4 w-4 mr-1" />
          Sync
        </Button>
        <Button variant="outline" onClick={handleExport} className="cursor-pointer">
          <IconDownload className="h-4 w-4 mr-1" />
          Export
        </Button>
      </div>
    </section>
  );
}
