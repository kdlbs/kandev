import { CanvasHostRoute } from "@/components/settings/canvas-host-route";
import { SettingsLayoutClient } from "@/components/settings/settings-layout-client";
import { WorkspaceCanvasesPage } from "@/components/settings/workspace-canvases-page";
import { WorkspaceSettingsShell } from "@/components/settings/workspaces/workspace-settings-shell";
import { AuthRouteRedirect } from "./spa-route-chrome";

type CanvasRouteProps =
  | { kind: "canvas"; canvasId: string }
  | { kind: "canvasSettings"; workspaceId: string };

export function CanvasRoute({ route, enabled }: { route: CanvasRouteProps; enabled: boolean }) {
  if (!enabled) return <AuthRouteRedirect />;
  if (route.kind === "canvas") return <CanvasHostRoute canvasId={route.canvasId} />;
  return (
    <SettingsLayoutClient>
      <WorkspaceSettingsShell workspaceId={route.workspaceId} activeTab="canvases">
        <WorkspaceCanvasesPage workspaceId={route.workspaceId} />
      </WorkspaceSettingsShell>
    </SettingsLayoutClient>
  );
}
