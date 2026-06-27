import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";
import { qk } from "@/lib/query/keys";
import { useInlineSlash } from "./use-inline-slash";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useInlineSlash", () => {
  afterEach(() => {
    cleanup();
  });

  it("reads available slash commands from TanStack Query without a Zustand store", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.sessionRuntime.availableCommands("session-1"), [
      { name: "pr-fixup", description: "Address PR review feedback" },
      { name: "internal", description: "Hidden (bundled)" },
    ]);

    const { result } = renderHook(
      () =>
        useInlineSlash(createRef<RichTextInputHandle>(), "", vi.fn(), { sessionId: "session-1" }),
      { wrapper: wrapperFor(queryClient) },
    );

    expect(result.current.commands.map((command) => command.label)).toEqual(["/pr-fixup"]);
  });
});
