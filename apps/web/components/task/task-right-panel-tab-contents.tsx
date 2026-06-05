"use client";

import { Badge } from "@kandev/ui/badge";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { TabsContent } from "@kandev/ui/tabs";
import { ShellTerminal } from "@/components/task/shell-terminal";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import type { RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

/** Commands tab content showing repository scripts */
export function CommandsTabContent({
  scripts,
  onRunCommand,
}: {
  scripts: RepositoryScript[];
  onRunCommand: (script: RepositoryScript) => void;
}) {
  return (
    <TabsContent value="commands" className="flex-1 min-h-0">
      <SessionPanelContent>
        <div className="grid gap-2">
          {scripts.map((script) => (
            <button
              key={script.id}
              type="button"
              onClick={() => onRunCommand(script)}
              className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-left hover:bg-muted cursor-pointer min-w-0"
            >
              <span className="flex-1 min-w-0 truncate text-xs">{script.name}</span>
              <Badge variant="secondary" className="shrink-0 font-mono text-xs max-w-[60%] min-w-0">
                <span className="truncate block">{script.command}</span>
              </Badge>
            </button>
          ))}
        </div>
      </SessionPanelContent>
    </TabsContent>
  );
}

/** Terminal tab contents (dev-server and shell terminals) */
export function TerminalTabContents({
  terminals,
  environmentId,
  devProcessId,
  devOutput,
  isStoppingDev,
}: {
  terminals: Terminal[];
  environmentId: string | null;
  devProcessId: string | null | undefined;
  devOutput: string | undefined;
  isStoppingDev: boolean;
}) {
  return (
    <>
      {terminals.map((terminal) => (
        <TabsContent key={terminal.id} value={terminal.id} className="flex-1 min-h-0">
          <SessionPanelContent className="p-0">
            {terminal.type === "dev-server" ? (
              <ShellTerminal
                key={devProcessId}
                processOutput={devOutput}
                processId={devProcessId ?? null}
                isStopping={isStoppingDev}
              />
            ) : (
              <PassthroughTerminal
                key={terminal.id}
                mode="shell"
                environmentId={environmentId}
                terminalId={terminal.id}
                label={terminal.type === "shell" ? terminal.label : undefined}
              />
            )}
          </SessionPanelContent>
        </TabsContent>
      ))}
    </>
  );
}
