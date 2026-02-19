"use client";

import { useEffect, useId } from "react";
import { useCommandRegistry } from "@/lib/commands/command-registry";
import type { CommandItem } from "@/lib/commands/types";

/**
 * Register commands that appear in the command panel.
 * Commands are automatically unregistered when the component unmounts.
 */
export function useRegisterCommands(commands: CommandItem[]) {
  const { register, unregister } = useCommandRegistry();
  const sourceId = useId();

  useEffect(() => {
    register(sourceId, commands);
    return () => unregister(sourceId);
  }, [sourceId, commands, register, unregister]);
}
