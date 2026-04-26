"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { IconRobot } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { SidebarCollapsibleSection } from "./sidebar-collapsible-section";

const statusDotColor: Record<string, string> = {
  idle: "bg-neutral-400",
  working: "bg-cyan-400 animate-pulse",
  paused: "bg-yellow-400",
  stopped: "bg-neutral-400 opacity-50",
  pending_approval: "bg-orange-400",
  error: "bg-red-400",
};

export function SidebarAgentsList() {
  const router = useRouter();
  const pathname = usePathname();
  const agents = useAppStore((s) => s.orchestrate.agentInstances);

  return (
    <SidebarCollapsibleSection
      label="Agents"
      onAdd={() => router.push("/orchestrate/agents")}
    >
      {agents.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">No agents yet</p>
      ) : (
        agents.map((agent) => {
          const href = `/orchestrate/agents/${agent.id}`;
          const isActive = pathname === href;
          return (
            <Link
              key={agent.id}
              href={href}
              className={cn(
                "flex items-center gap-2.5 px-3 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
                isActive
                  ? "bg-accent text-foreground"
                  : "text-foreground/80 hover:bg-accent/50",
              )}
            >
              <IconRobot className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <span className="flex-1 truncate">{agent.name}</span>
              <span
                className={cn(
                  "inline-block h-2 w-2 rounded-full shrink-0",
                  statusDotColor[agent.status] ?? "bg-neutral-400",
                )}
              />
            </Link>
          );
        })
      )}
    </SidebarCollapsibleSection>
  );
}
