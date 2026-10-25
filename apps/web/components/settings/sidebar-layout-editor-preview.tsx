"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";

export function SidebarDraftPreview({
  nodes,
  t,
}: {
  nodes: ProjectedSidebarNode[];
  t: (key: string) => string;
}) {
  return (
    <Card data-testid="sidebar-layout-preview">
      <CardHeader>
        <CardTitle>{t("settings:sidebarPreview")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {nodes.map((node) => (
          <div key={node.id} className="rounded-md border p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <node.icon className="h-4 w-4" />
              <span className="truncate">{node.label}</span>
              {node.kind === "shortcuts" && (
                <span className="text-xs text-muted-foreground">{node.shortcuts.length}</span>
              )}
            </div>
            {node.kind === "shortcuts" && node.shortcuts.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {node.shortcuts.map((shortcut) => {
                  const Icon = shortcut.icon;
                  return (
                    <Icon
                      key={shortcut.id}
                      className="h-4 w-4 text-muted-foreground"
                      aria-label={shortcut.label}
                    />
                  );
                })}
              </div>
            )}
          </div>
        ))}
        {nodes.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("settings:sidebarPreviewEmpty")}</p>
        )}
      </CardContent>
    </Card>
  );
}
