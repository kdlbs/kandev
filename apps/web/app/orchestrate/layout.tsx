import { OrchestrateSidebar } from "./components/orchestrate-sidebar";

export default function OrchestrateLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen">
      <OrchestrateSidebar />
      <main className="flex-1 min-w-0 overflow-y-auto">{children}</main>
    </div>
  );
}
