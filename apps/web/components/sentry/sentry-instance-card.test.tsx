import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { SentryConfig } from "@/lib/types/sentry";
import { SentryInstanceCard } from "./sentry-instance-card";

function instance(id: string, name: string): SentryConfig {
  return {
    id,
    workspaceId: "workspace-1",
    name,
    authMethod: "auth_token",
    url: "https://sentry.example.com",
    hasSecret: false,
    lastOk: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

afterEach(() => cleanup());

describe("SentryInstanceCard", () => {
  it("gives each instance action a distinct accessible name", () => {
    render(
      <>
        <SentryInstanceCard
          instance={instance("instance-1", "Production")}
          onEdit={() => {}}
          onDelete={() => {}}
        />
        <SentryInstanceCard
          instance={instance("instance-2", "Self-hosted")}
          onEdit={() => {}}
          onDelete={() => {}}
        />
      </>,
    );

    expect(screen.getByRole("button", { name: "Edit Production Sentry instance" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete Self-hosted Sentry instance" })).toBeTruthy();
  });
});
