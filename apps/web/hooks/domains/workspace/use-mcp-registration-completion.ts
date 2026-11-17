"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";

export type MCPRegistrationReceipt = {
  workspaceId: string;
  definitionId: string;
  displayName: string;
};
export type MCPRegistrationResult =
  | { status: "saved"; definition: MCPServerDefinition }
  | { status: "uncertain" | "ignored" };

export function useMCPRegistrationCompletion(workspaceId: string) {
  const [receipt, setReceipt] = useState<MCPRegistrationReceipt | null>(null);
  const [uncertain, setUncertain] = useState(false);
  const [pending, setPending] = useState(false);
  const gate = useRef({ generation: 0, pending: false, uncertain: false });
  const workspaceRef = useRef(workspaceId);
  workspaceRef.current = workspaceId;
  useEffect(() => {
    gate.current = { generation: gate.current.generation + 1, pending: false, uncertain: false };
    setReceipt(null);
    setUncertain(false);
    setPending(false);
    return () => {
      gate.current.generation += 1;
    };
  }, [workspaceId]);

  const register = useCallback(
    async (operation: () => Promise<MCPServerDefinition>): Promise<MCPRegistrationResult> => {
      if (workspaceRef.current !== workspaceId || gate.current.pending || gate.current.uncertain)
        return { status: "ignored" };
      const generation = gate.current.generation;
      const isCurrent = () =>
        workspaceRef.current === workspaceId && gate.current.generation === generation;
      gate.current.pending = true;
      setPending(true);
      let definition: MCPServerDefinition;
      try {
        definition = await operation();
        if (!definition?.id || definition.workspace_id !== workspaceId) throw new TypeError();
      } catch (cause) {
        if (!isCurrent()) return { status: "ignored" };
        if (cause instanceof ApiError && cause.status >= 400 && cause.status < 500) throw cause;
        gate.current.uncertain = true;
        setUncertain(true);
        setReceipt(null);
        return { status: "uncertain" };
      } finally {
        if (isCurrent()) {
          gate.current.pending = false;
          setPending(false);
        }
      }
      if (!isCurrent()) return { status: "ignored" };
      setReceipt({
        workspaceId,
        definitionId: definition.id,
        displayName: definition.display_name,
      });
      return { status: "saved", definition };
    },
    [workspaceId],
  );

  const reviewed = useCallback(() => {
    gate.current.uncertain = false;
    setUncertain(false);
  }, []);
  return {
    receipt: receipt?.workspaceId === workspaceId ? receipt : null,
    uncertain,
    pending,
    register,
    reviewed,
    dismiss: () => setReceipt(null),
  };
}
