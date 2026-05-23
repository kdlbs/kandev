import React from "react";
import { render, type RenderOptions, type RenderResult } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * Options for renderWithQueryClient.
 * `client` defaults to a test-friendly QueryClient with retry and gcTime disabled.
 */
export interface RenderWithQueryOptions extends Omit<RenderOptions, "wrapper"> {
  client?: QueryClient;
}

export interface RenderWithQueryResult extends RenderResult {
  client: QueryClient;
}

/**
 * Creates a test-scoped QueryClient with:
 * - retry: false — no retries so tests fail fast on error
 * - gcTime: 0 — garbage-collect immediately so each test starts clean
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

/**
 * Renders `ui` inside a QueryClientProvider.
 *
 * Usage:
 *   const { client, getByText } = renderWithQueryClient(<MyComponent />);
 *   await waitFor(() => expect(getByText("hello")).toBeInTheDocument());
 *
 * Pass a custom `client` to share state across multiple renders in one test:
 *   const client = createTestQueryClient();
 *   renderWithQueryClient(<A />, { client });
 *   renderWithQueryClient(<B />, { client });
 */
export function renderWithQueryClient(
  ui: React.ReactElement,
  { client, ...options }: RenderWithQueryOptions = {},
): RenderWithQueryResult {
  const queryClient = client ?? createTestQueryClient();

  function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }

  const result = render(ui, { wrapper: Wrapper, ...options });
  return { client: queryClient, ...result };
}
