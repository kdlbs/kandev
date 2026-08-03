import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MessageListProps } from "./message-list-shared";

vi.mock("./message-list-native", async () => {
  const React = await import("react");
  return {
    NativeMessageList: React.forwardRef<HTMLDivElement, Record<string, unknown>>((_props, ref) =>
      React.createElement("div", { ref, "data-testid": "native-message-list" }),
    ),
  };
});

import { MessageList } from "./message-list";

afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
  vi.restoreAllMocks();
});

describe("MessageList", () => {
  it("keeps the transcript on the native renderer when the old Virtuoso override is requested", () => {
    window.history.replaceState({}, "", "/t/session-a?renderer=virtuoso");

    render(<MessageList {...({} as unknown as MessageListProps)} />);

    expect(screen.getByTestId("native-message-list")).toBeTruthy();
  });
});
