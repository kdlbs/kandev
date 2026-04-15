"use client";

import { useEffect } from "react";
import { useToast } from "@/components/toast-provider";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";

export function useMermaidErrorToast(): void {
  const { toast } = useToast();

  useEffect(() => {
    const handler = () => {
      toast({ title: "Failed to render diagram", variant: "error" });
    };
    document.addEventListener(MERMAID_ERROR_EVENT, handler);
    return () => document.removeEventListener(MERMAID_ERROR_EVENT, handler);
  }, [toast]);
}
