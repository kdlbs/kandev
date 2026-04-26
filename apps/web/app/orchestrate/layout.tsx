import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkspaceRail } from "./components/workspace-rail";
import { OrchestrateSidebar } from "./components/orchestrate-sidebar";

export default function OrchestrateLayout({ children }: { children: React.ReactNode }) {
  return (
    <TooltipProvider>
      <div className="flex h-screen">
        <WorkspaceRail />
        <OrchestrateSidebar />
        <main className="flex-1 min-w-0 overflow-y-auto">{children}</main>
      </div>
    </TooltipProvider>
  );
}
