import { useState } from "react";
import { fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";

const rendererSpy = vi.fn();

vi.mock("@/components/task/chat/message-renderer", () => ({
  MessageRenderer: (props: { onOpenFile?: unknown }) => {
    rendererSpy(props);
    return <div data-testid="renderer" />;
  },
}));
vi.mock("@/components/task/chat/messages/turn-group-message", () => ({
  TurnGroupMessage: () => <div data-testid="turn-group" />,
}));
vi.mock("@/components/session/prepare-progress", () => ({
  PrepareProgress: () => <div data-testid="prepare" />,
}));

import { MessageItem } from "./message-list-shared";

const item: RenderItem = { type: "message", message: { id: "m1" } as Message };
const noop = () => {};
const perm = new Map<string, Message>();
const kids = new Map<string, Message[]>();

function row(onOpenFile: (p: string) => void) {
  return (
    <MessageItem
      item={item}
      sessionId="s1"
      permissionsByToolCallId={perm}
      childrenByParentToolCallId={kids}
      taskId="t1"
      worktreePath="/wt"
      onOpenFile={onOpenFile}
      isLastGroup={false}
      isTurnActive={false}
      onScrollToMessage={noop}
    />
  );
}

function Harness({ onOpenFile }: { onOpenFile: (p: string) => void }) {
  const [, setTick] = useState(0);
  return (
    <div>
      <button onClick={() => setTick((t) => t + 1)}>tick</button>
      {row(onOpenFile)}
    </div>
  );
}

describe("MessageItem memo boundary", () => {
  afterEach(() => {
    rendererSpy.mockClear();
  });

  it("does not re-render the row when the parent re-renders with stable props", () => {
    const { getByText } = render(<Harness onOpenFile={noop} />);
    expect(rendererSpy).toHaveBeenCalledTimes(1);
    fireEvent.click(getByText("tick"));
    fireEvent.click(getByText("tick"));
    expect(rendererSpy).toHaveBeenCalledTimes(1); // memo bailed on stable props
  });

  it("re-renders the row when onOpenFile identity changes (stability requirement)", () => {
    const { rerender } = render(row(() => {}));
    expect(rendererSpy).toHaveBeenCalledTimes(1);
    rerender(row(() => {}));
    expect(rendererSpy).toHaveBeenCalledTimes(2); // fresh callback ref breaks memo
  });
});
