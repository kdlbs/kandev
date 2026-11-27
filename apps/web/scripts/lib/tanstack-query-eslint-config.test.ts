import path from "node:path";

import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const WEB_DIR = path.resolve(import.meta.dirname, "../..");
const eslint = new ESLint({
  cwd: WEB_DIR,
  overrideConfigFile: path.join(WEB_DIR, "eslint.config.mjs"),
});

async function lintFixture(fileName: string, source: string[]) {
  const [result] = await eslint.lintText(source.join("\n"), {
    filePath: path.join(WEB_DIR, fileName),
  });
  return result.messages.filter((message) => message.ruleId?.startsWith("@tanstack/query/"));
}

describe("TanStack Query ESLint key and result rules", () => {
  it("keeps the current system info Query integration compliant", async () => {
    const results = await eslint.lintFiles([
      "hooks/domains/system/use-system-info.ts",
      "components/system-info-query-provider.tsx",
    ]);
    const messages = results
      .flatMap((result) => result.messages)
      .filter((message) => message.ruleId?.startsWith("@tanstack/query/"));

    expect(messages).toEqual([]);
  }, 15000);

  it("requires query keys to include values read by query functions", async () => {
    const messages = await lintFixture("hooks/domains/system/query-eslint-config.test.ts", [
      'import { useQuery } from "@tanstack/react-query";',
      "export function useTodo(todoId: string) {",
      "  return useQuery({",
      '    queryKey: ["todo"],',
      "    queryFn: () => todoId,",
      "  });",
      "}",
    ]);

    expect(messages).toContainEqual(
      expect.objectContaining({
        ruleId: "@tanstack/query/exhaustive-deps",
        severity: 2,
      }),
    );
  });

  it("warns when a query result is rest destructured", async () => {
    const messages = await lintFixture("hooks/domains/system/query-eslint-config.test.ts", [
      'import { useQuery } from "@tanstack/react-query";',
      "export function useTodo() {",
      "  const { data, ...rest } = useQuery({",
      '    queryKey: ["todo"],',
      "    queryFn: async () => 1,",
      "  });",
      "  return { data, ...rest };",
      "}",
    ]);

    expect(messages).toContainEqual(
      expect.objectContaining({
        ruleId: "@tanstack/query/no-rest-destructuring",
        severity: 1,
      }),
    );
  });
});

describe("TanStack Query ESLint client rules", () => {
  it("reports an unstable query client and query hook dependency", async () => {
    const messages = await lintFixture("components/query-eslint-config.test.tsx", [
      'import { useCallback } from "react";',
      'import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";',
      "export function App() {",
      "  const client = new QueryClient();",
      '  const query = useQuery({ queryKey: ["todo"], queryFn: async () => 1 });',
      "  const refresh = useCallback(() => query.refetch(), [query]);",
      "  return <QueryClientProvider client={client}><button onClick={refresh} /></QueryClientProvider>;",
      "}",
    ]);

    expect(messages).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          ruleId: "@tanstack/query/stable-query-client",
          severity: 2,
        }),
        expect.objectContaining({
          ruleId: "@tanstack/query/no-unstable-deps",
          severity: 2,
        }),
      ]),
    );
  });

  it("accepts a stable client and a destructured query result", async () => {
    const messages = await lintFixture("components/query-eslint-config.test.tsx", [
      'import { useCallback, useState } from "react";',
      'import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";',
      "export function App() {",
      "  const [client] = useState(() => new QueryClient());",
      '  const { refetch } = useQuery({ queryKey: ["todo"], queryFn: async () => 1 });',
      "  const refresh = useCallback(() => refetch(), [refetch]);",
      "  return <QueryClientProvider client={client}><button onClick={refresh} /></QueryClientProvider>;",
      "}",
    ]);

    expect(messages).toEqual([]);
  });
});
