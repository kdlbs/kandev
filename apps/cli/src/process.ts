import kill from "tree-kill";

export type ChildLike = { pid?: number | undefined };

export function createProcessSupervisor(): {
  children: ChildLike[];
  shutdown: (reason: string) => Promise<void>;
  attachSignalHandlers: () => void;
} {
  let shuttingDown = false;
  const children: ChildLike[] = [];

  const shutdown = async (reason: string) => {
    if (shuttingDown) return;
    shuttingDown = true;
    console.log(`[kandev] shutting down (${reason})...`);
    await Promise.all(
      children
        .filter((child) => child.pid)
        .map((child) => new Promise<void>((resolve) => kill(child.pid as number, "SIGTERM", () => resolve()))),
    );
  };

  const onSignal = (signal: NodeJS.Signals) => {
    void shutdown(`signal ${signal}`).then(() => process.exit(0));
  };

  const attachSignalHandlers = () => {
    process.on("SIGINT", onSignal);
    process.on("SIGTERM", onSignal);
  };

  return { children, shutdown, attachSignalHandlers };
}
