import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { CopyMoveSecretDialog } from "./copy-move-secret-dialog";

type Ticket = {
  url: string;
  init?: RequestInit;
  promise: Promise<Response>;
  resolve: (response: Response) => void;
  settled: boolean;
};
let tickets: Ticket[];
const nameInvalidAttribute = "aria-invalid";
const destinationName = "Beta";
const timestamp = "2026-10-06T00:00:00Z";
const source: SecretListItem = {
  id: "source",
  name: "Source key",
  scope: "workspace",
  workspace_id: "alpha",
  has_value: true,
  created_at: timestamp,
  updated_at: timestamp,
};
const workspaces = [
  { id: "alpha", name: "Alpha", owner_id: "user", created_at: timestamp, updated_at: timestamp },
  {
    id: "beta",
    name: destinationName,
    owner_id: "user",
    created_at: timestamp,
    updated_at: timestamp,
  },
];

beforeEach(() => {
  tickets = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      let resolve!: Ticket["resolve"];
      const promise = new Promise<Response>((yes) => {
        resolve = yes;
      });
      tickets.push({ url: String(input), init, promise, resolve, settled: false });
      return promise;
    }),
  );
});

async function respond(index: number, body: unknown = [], status = 200) {
  const ticket = tickets[index];
  expect(ticket).toBeDefined();
  await act(async () => {
    ticket.settled = true;
    ticket.resolve(new Response(JSON.stringify(body), { status }));
    await ticket.promise;
  });
}

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const ticket of tickets.filter((item) => !item.settled)) {
      ticket.settled = true;
      ticket.resolve(new Response("[]", { status: 200 }));
    }
    await Promise.allSettled(tickets.map((ticket) => ticket.promise));
  });
  vi.unstubAllGlobals();
});

function renderDialog() {
  const onCompleted = vi.fn();
  const onClose = vi.fn();
  const tree = (open = true) => (
    <StateProvider initialState={{ workspaces: { items: workspaces, activeId: "alpha" } }}>
      <CopyMoveSecretDialog
        secret={source}
        originToken="Alpha"
        open={open}
        onClose={onClose}
        onCompleted={onCompleted}
      />
    </StateProvider>
  );
  return { view: render(tree()), tree, onCompleted };
}

async function selectDestination(name: string) {
  fireEvent.click(screen.getByRole("combobox", { name: "Destination" }));
  fireEvent.click(await screen.findByRole("option", { name }));
}

function setName(name: string) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: name } });
}

function nameInput() {
  return screen.getByLabelText("Name") as HTMLInputElement;
}

function primary(mode = "Copy") {
  return screen.getByRole("button", { name: mode }) as HTMLButtonElement;
}

function destinationSecret(name: string): SecretListItem {
  return { ...source, id: `destination-${name}`, name, workspace_id: "beta" };
}

describe("cancelled destination lookup recovery", () => {
  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.9
  it("restores the rendered duplicate pre-check after a pending roundtrip", async () => {
    renderDialog();
    setName(" Taken ");
    await selectDestination(destinationName);
    expect(tickets).toHaveLength(1);
    expect(primary().disabled).toBe(false);
    await selectDestination("Global");
    await selectDestination(destinationName);
    expect(tickets).toHaveLength(2);
    await respond(1, [destinationSecret("Taken")]);
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("true");
    const errorId = nameInput().getAttribute("aria-describedby");
    expect(errorId).toBe("copy-move-name-error");
    expect(document.getElementById(errorId!)?.textContent).toBe(
      "A secret named Taken already exists in this destination.",
    );
    expect(primary().disabled).toBe(true);
    await respond(0, [destinationSecret("Abandoned")]);
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("true");
    setName("Available");
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("false");
    expect(primary().disabled).toBe(false);
  });

  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.10
  it("reuses a completed lookup across the rendered destination roundtrip", async () => {
    renderDialog();
    setName("Taken");
    await selectDestination(destinationName);
    await respond(0, [destinationSecret("Taken")]);
    expect(primary().disabled).toBe(true);
    await selectDestination("Global");
    expect(primary().disabled).toBe(false);
    await selectDestination(destinationName);
    expect(tickets).toHaveLength(1);
    expect(primary().disabled).toBe(true);
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("true");
  });

  it("refreshes names when the mounted dialog is reopened", async () => {
    const { view, tree } = renderDialog();
    await selectDestination(destinationName);
    await respond(0);
    view.rerender(tree(false));
    view.rerender(tree());
    setName("New duplicate");
    await selectDestination(destinationName);
    expect(tickets).toHaveLength(2);
    await respond(1, [destinationSecret("New duplicate")]);
    expect(primary().disabled).toBe(true);
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("true");
  });
});

describe("destination lookup transfer fallbacks", () => {
  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.11
  it("allows an otherwise valid transfer after the current names lookup fails", async () => {
    const { onCompleted } = renderDialog();
    await selectDestination(destinationName);
    await respond(0, { error: "Unavailable" }, 500);
    setName(" Available ");
    expect(primary().disabled).toBe(false);
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("false");
    fireEvent.click(primary());
    expect(tickets[1].url).toContain("/api/v1/secrets/source/copy?workspace_id=alpha");
    expect(tickets[1].init?.method).toBe("POST");
    expect(JSON.parse(String(tickets[1].init?.body))).toEqual({
      target_scope: "workspace",
      target_workspace_id: "beta",
      name: "Available",
    });
    const copied = destinationSecret("Available");
    await respond(1, copied, 201);
    expect(onCompleted).toHaveBeenCalledWith(copied, "copy");
  });

  it.each([409, 500])("preserves actual HTTP %s transfer errors", async (status) => {
    const { onCompleted } = renderDialog();
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    await selectDestination(destinationName);
    await respond(0);
    setName("Taken");
    fireEvent.click(primary("Move"));
    expect(tickets[1].url).toContain("/api/v1/secrets/source/move?workspace_id=alpha");
    expect(JSON.parse(String(tickets[1].init?.body))).toEqual({
      target_scope: "workspace",
      target_workspace_id: "beta",
      name: "Taken",
    });
    await respond(1, { error: "Backend rejection" }, status);
    expect(onCompleted).not.toHaveBeenCalled();
    if (status === 409) {
      expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("true");
      expect(
        screen.getByText("A secret named Taken already exists in this destination."),
      ).toBeTruthy();
      expect(primary("Move").disabled).toBe(true);
    } else {
      expect(screen.getByRole("alert").textContent).toBe("Could not copy or move the secret.");
      expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("false");
      expect(primary("Move").disabled).toBe(false);
    }
    await selectDestination("Global");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(nameInput().getAttribute(nameInvalidAttribute)).toBe("false");
    fireEvent.click(primary("Move"));
    expect(JSON.parse(String(tickets[2].init?.body))).toEqual({
      target_scope: "global",
      name: "Taken",
    });
    const moved = { ...destinationSecret("Taken"), scope: "global", workspace_id: undefined };
    await respond(2, moved, 201);
    expect(onCompleted).toHaveBeenCalledWith(moved, "move");
  });
});
