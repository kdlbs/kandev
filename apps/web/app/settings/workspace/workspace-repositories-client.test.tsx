import { StrictMode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { repositoryDiscoveryCoordinator } from "@/hooks/domains/workspace/use-repository-discovery";
import { i18n } from "@/lib/i18n";
import {
  workspaceId,
  type RepositoryPathValidationResponse,
  type Workspace,
} from "@/lib/types/http";
import { AddLocalRepositoryDialog } from "./workspace-add-local-repository-dialog";
import { useWorkspaceRepositoriesPage } from "./workspace-repositories-client";

type Page = ReturnType<typeof useWorkspaceRepositoriesPage>;
type Outcome = "success" | "invalid" | "rejection";
type Ticket = {
  url: URL;
  promise: Promise<Response>;
  finish: (outcome: Outcome, path?: string) => void;
};
const ALPHA = "/repos/alpha";
const BETA = "/repos/beta";
const CANONICAL = "/canonical/alpha";
const ERROR = "validation transport refused";
const INVALID = "not a Git working tree";
const DISCOVERED = { name: "Discovered", path: "/discovered/repo", default_branch: "release" };
let latest: Page;
let tickets: Ticket[];
let callers: Promise<void>[];
let urls: { url: URL; method: string }[];
let desktop: boolean;
let reenter: (() => void) | undefined;

function workspace(id = "workspace-a"): Workspace {
  return {
    id: workspaceId(id),
    name: id,
    owner_id: "owner",
    created_at: "",
    updated_at: "",
    acp_idle_suspension_enabled: false,
    acp_idle_timeout_minutes: 30,
  };
}

function json(value: unknown): Response {
  return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
}

function validationTicket(url: URL): Ticket {
  let resolve!: (response: Response) => void;
  let reject!: (reason: Error) => void;
  let finished = false;
  const promise = new Promise<Response>((accept, refuse) => {
    resolve = accept;
    reject = refuse;
  });
  return {
    url,
    promise,
    finish(outcome, path = CANONICAL) {
      if (finished) return;
      finished = true;
      if (outcome === "rejection") reject(new Error(ERROR));
      else
        resolve(
          json({
            path,
            exists: true,
            is_git: outcome === "success",
            allowed: false,
            message: outcome === "invalid" ? INVALID : undefined,
          } satisfies RepositoryPathValidationResponse),
        );
    },
  };
}

function fetchTransport(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = new URL(String(input), "http://localhost");
  urls.push({ url, method: init?.method ?? "GET" });
  if (url.pathname.endsWith("/repositories/validate")) {
    const ticket = validationTicket(url);
    tickets.push(ticket);
    const callback = reenter;
    reenter = undefined;
    callback?.();
    return ticket.promise;
  }
  if (url.pathname.endsWith("/repositories/discovery"))
    return Promise.resolve(
      json({
        roots: ["/discovered"],
        repositories: [DISCOVERED],
        total: 1,
        desktop_runtime: desktop,
        root_states: [],
        scan_time: new Date().toISOString(),
        cached: true,
        refreshing: false,
        home_confirmation_required: false,
        failed_roots: [],
      }),
    );
  throw new Error(`Unexpected transport: ${url.pathname}`);
}

function Probe({ current }: { current: Workspace | null }) {
  latest = useWorkspaceRepositoriesPage(current, []);
  const store = useAppStoreApi();
  return (
    <>
      <button onClick={latest.openDialog}>Open</button>
      <output data-testid="store-workspace">{store.getState().workspaces.activeId}</output>
      <AddLocalRepositoryDialog state={latest} />
      <output data-testid="drafts">{JSON.stringify(latest.repositoryItems)}</output>
    </>
  );
}

async function mount(current: Workspace | null = workspace(), strict = false) {
  const tree = (value: Workspace | null) => (
    <StateProvider>
      <ToastProvider>
        <Probe current={value} />
      </ToastProvider>
    </StateProvider>
  );
  const view = render(strict ? <StrictMode>{tree(current)}</StrictMode> : tree(current));
  fireEvent.click(screen.getByRole("button", { name: "Open" }));
  if (current) await waitFor(() => expect(latest.isDiscovering).toBe(false));
  return {
    ...view,
    changeWorkspace(value: Workspace | null) {
      view.rerender(tree(value));
    },
  };
}

function input(): HTMLInputElement {
  return screen.getByPlaceholderText("/absolute/path/to/repository");
}
function confirm(): HTMLButtonElement {
  return screen.getByRole("button", { name: i18n.t("workspaces:useRepository") });
}
function edit(value = ALPHA) {
  fireEvent.change(input(), { target: { value } });
}
function start(handler = latest.handleValidateManualPath): Promise<void> {
  let call!: Promise<void>;
  act(() => {
    call = handler();
    callers.push(call);
  });
  return call;
}
async function settle(index: number, outcome: Outcome = "success", path = CANONICAL) {
  await act(async () => {
    tickets[index].finish(outcome, path);
    await Promise.resolve();
  });
}
function idle() {
  expect(latest.manualValidation.status).toBe("idle");
  expect(latest.isValidating).toBe(false);
  expect(confirm().disabled).toBe(true);
  expect(latest.repositoryItems).toEqual([]);
}
function noCreation() {
  expect(urls.filter((row) => row.method === "POST")).toEqual([]);
}

beforeEach(async () => {
  tickets = [];
  callers = [];
  urls = [];
  desktop = false;
  reenter = undefined;
  repositoryDiscoveryCoordinator.dispose();
  await i18n.changeLanguage("en");
  vi.stubGlobal("fetch", vi.fn(fetchTransport));
});
afterEach(async () => {
  await act(async () => {
    tickets.forEach((ticket) => ticket.finish("success"));
    await Promise.allSettled(callers);
  });
  cleanup();
  repositoryDiscoveryCoordinator.dispose();
  vi.unstubAllGlobals();
});

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-001.9, AC-WORKSPACES-LOCAL-REPOSITORIES-001.11
it.each<Outcome>(["success", "invalid", "rejection"])(
  "manual repository validation retires %s after a real input edit",
  async (outcome) => {
    await mount();
    edit();
    const call = start();
    expect(input().disabled).toBe(false);
    expect(confirm().disabled).toBe(true);
    edit(BETA);
    await settle(0, outcome);
    expect(await call).toBeUndefined();
    expect(input().value).toBe(BETA);
    idle();
    act(() => latest.handleConfirmLocalRepository());
    expect(latest.repositoryItems).toEqual([]);
    noCreation();
  },
);

it("manual repository validation does not revive an A-B-A input", async () => {
  await mount();
  edit();
  const call = start();
  edit(BETA);
  edit(ALPHA);
  await settle(0);
  await call;
  idle();
});

it.each(["pending", "accepted"])(
  "manual repository validation retires an %s result on edit",
  async (phase) => {
    await mount();
    edit();
    const call = start();
    if (phase === "accepted") {
      await settle(0);
      await call;
    }
    edit(BETA);
    idle();
    edit(ALPHA);
    idle();
    await settle(0);
    await call;
    idle();
  },
);

it.each(["cancel", "dismiss", "reopened blank"])(
  "manual repository validation retires work when the dialog closes by %s",
  async (mode) => {
    await mount();
    edit();
    const call = start();
    if (mode === "cancel")
      fireEvent.click(screen.getByRole("button", { name: i18n.t("common:cancel") }));
    else fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(latest.localRepoDialogOpen).toBe(false);
    if (mode === "reopened blank") fireEvent.click(screen.getByRole("button", { name: "Open" }));
    await settle(0);
    await call;
    if (mode !== "reopened blank") fireEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(input().value).toBe("");
    idle();
  },
);

it.each<Outcome>(["success", "rejection"])(
  "manual repository validation ignores retired %s in a reopened visit",
  async (outcome) => {
    await mount();
    edit();
    const old = start();
    fireEvent.click(screen.getByRole("button", { name: i18n.t("common:cancel") }));
    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    edit(BETA);
    const current = start();
    await settle(0, outcome);
    expect(await old).toBeUndefined();
    expect(latest.isValidating).toBe(true);
    expect(confirm().disabled).toBe(true);
    await settle(1, "success", BETA);
    await current;
    expect(latest.canSave).toBe(true);
    fireEvent.click(confirm());
    expect(latest.repositoryItems[0].local_path).toBe(BETA);
  },
);

it.each(["replacement", "roundtrip", "missing"])(
  "manual repository validation retires a %s workspace context",
  async (mode) => {
    const view = await mount();
    edit();
    const oldConfirm = latest.handleConfirmLocalRepository;
    const oldValidate = latest.handleValidateManualPath;
    const call = start();
    view.changeWorkspace(mode === "missing" ? null : workspace("workspace-b"));
    if (mode === "roundtrip") view.changeWorkspace(workspace());
    await settle(0);
    await call;
    act(() => oldConfirm());
    const stale = start(oldValidate);
    expect(tickets).toHaveLength(1);
    await stale;
    expect(latest.canSave).toBe(false);
    expect(latest.isValidating).toBe(false);
    expect(latest.manualValidation.status).toBe("idle");
    expect(latest.repositoryItems).toEqual([]);
  },
);

it.each<Outcome>(["success", "rejection"])(
  "manual repository validation settles %s after unmount without reusing callbacks",
  async (outcome) => {
    const view = await mount();
    edit();
    const oldValidate = latest.handleValidateManualPath;
    const oldConfirm = latest.handleConfirmLocalRepository;
    const oldClose = latest.setLocalRepoDialogOpen;
    const call = start();
    view.unmount();
    await settle(0, outcome);
    expect(await call).toBeUndefined();
    await mount();
    edit(BETA);
    const stale = start(oldValidate);
    expect(tickets).toHaveLength(1);
    await stale;
    act(() => {
      oldConfirm();
      oldClose(false);
    });
    expect(input().value).toBe(BETA);
    idle();
  },
);

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-001.10
it.each<Outcome>(["success", "invalid", "rejection"])(
  "manual repository validation keeps newest pending while older %s settles",
  async (outcome) => {
    await mount();
    edit();
    const handler = latest.handleValidateManualPath;
    const older = start(handler);
    const newer = start(handler);
    await settle(0, outcome);
    await older;
    expect(latest.isValidating).toBe(true);
    expect(confirm().disabled).toBe(true);
    expect(latest.manualValidation.status).toBe("loading");
    await settle(1);
    await newer;
    expect(latest.manualValidation.path).toBe(CANONICAL);
    expect(latest.canSave).toBe(true);
  },
);

it.each<Outcome>(["success", "invalid", "rejection"])(
  "manual repository validation ignores older settlement after newest %s",
  async (outcome) => {
    await mount();
    edit();
    const handler = latest.handleValidateManualPath;
    const older = start(handler);
    const newer = start(handler);
    await settle(1, outcome, "/newest");
    await newer;
    const accepted = latest.manualValidation;
    await settle(0, "success", "/obsolete");
    await older;
    expect(latest.manualValidation).toEqual(accepted);
    expect(latest.isValidating).toBe(false);
    expect(latest.canSave).toBe(outcome === "success");
  },
);

it("manual repository validation reserves newest authority during transport reentry", async () => {
  await mount();
  edit();
  const handler = latest.handleValidateManualPath;
  let reentrant!: Promise<void>;
  reenter = () => {
    reentrant = handler();
    callers.push(reentrant);
  };
  const first = start(handler);
  expect(tickets).toHaveLength(2);
  await settle(1, "success", "/reentrant");
  await reentrant;
  await settle(0, "rejection");
  await first;
  expect(latest.manualValidation.path).toBe("/reentrant");
  expect(latest.canSave).toBe(true);
  expect(latest.isValidating).toBe(false);
});

it.each(["edit", "selection", "close", "workspace", "attempt"])(
  "manual repository validation rejects retained handlers after %s retirement",
  async (mode) => {
    const view = await mount();
    edit();
    const first = start();
    await settle(0);
    await first;
    const oldConfirm = latest.handleConfirmLocalRepository;
    const oldValidate = latest.handleValidateManualPath;
    const oldClose = latest.setLocalRepoDialogOpen;
    if (mode === "edit") edit(BETA);
    if (mode === "selection") fireEvent.click(screen.getByRole("button", { name: /Discovered/ }));
    if (mode === "close") {
      fireEvent.click(screen.getByRole("button", { name: i18n.t("common:cancel") }));
      fireEvent.click(screen.getByRole("button", { name: "Open" }));
      edit(BETA);
    }
    if (mode === "workspace") {
      view.changeWorkspace(workspace("workspace-b"));
      fireEvent.click(screen.getByRole("button", { name: "Open" }));
      edit(BETA);
    }
    if (mode === "attempt") {
      const second = start();
      await settle(1, "success", "/newest");
      await second;
    }
    const before = tickets.length;
    const stale = mode !== "attempt" ? start(oldValidate) : undefined;
    expect(tickets).toHaveLength(before);
    await stale;
    act(() => {
      oldConfirm();
      if (mode !== "attempt") oldClose(false);
    });
    expect(latest.repositoryItems).toEqual([]);
    expect(latest.localRepoDialogOpen).toBe(true);
  },
);

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-001.1, AC-WORKSPACES-LOCAL-REPOSITORIES-001.3, AC-WORKSPACES-LOCAL-REPOSITORIES-001.11
it("manual repository validation confirms the canonical current path without persistence", async () => {
  await mount();
  edit(`  ${ALPHA}  `);
  fireEvent.click(screen.getByRole("button", { name: i18n.t("workspaces:validate") }));
  expect(tickets[0].url.searchParams.get("path")).toBe(ALPHA);
  await settle(0);
  expect(confirm().disabled).toBe(false);
  expect(screen.getByText(i18n.t("workspaces:validGitRepository"))).toBeTruthy();
  edit(` ${ALPHA} `);
  expect(confirm().disabled).toBe(false);
  fireEvent.click(confirm());
  expect(latest.repositoryItems).toHaveLength(1);
  expect(latest.repositoryItems[0]).toMatchObject({
    local_path: CANONICAL,
    workspace_id: "workspace-a",
    name: "alpha",
  });
  expect(latest.localRepoDialogOpen).toBe(false);
  noCreation();
});

it.each<Outcome>(["invalid", "rejection"])(
  "manual repository validation preserves a current %s and caller settlement",
  async (outcome) => {
    await mount();
    edit();
    const call = start();
    await settle(0, outcome);
    expect(await call).toBeUndefined();
    expect(latest.isValidating).toBe(false);
    expect(confirm().disabled).toBe(true);
    expect(screen.getByText(outcome === "invalid" ? INVALID : ERROR)).toBeTruthy();
    act(() => latest.handleConfirmLocalRepository());
    expect(latest.repositoryItems).toEqual([]);
  },
);

it("manual repository validation keeps a discovered selection with actual desktop controls", async () => {
  desktop = true;
  await mount();
  edit();
  const pending = start();
  expect(latest.desktopRuntime).toBe(true);
  expect(screen.getByTestId("discovery-root-controls")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Discovered/ }));
  expect(input().value).toBe("");
  expect(confirm().disabled).toBe(false);
  await settle(0, "rejection");
  await pending;
  expect(latest.manualValidation.status).toBe("idle");
  expect(confirm().disabled).toBe(false);
  fireEvent.click(confirm());
  expect(latest.repositoryItems[0]).toMatchObject({
    local_path: DISCOVERED.path,
    name: DISCOVERED.name,
    default_branch: DISCOVERED.default_branch,
  });
  noCreation();
});

it.each([true, false])(
  "manual repository validation rejects empty input with workspace %s",
  async (present) => {
    await mount(present ? workspace() : null);
    edit("   ");
    await start();
    act(() => latest.handleConfirmLocalRepository());
    expect(tickets).toHaveLength(0);
    expect(latest.repositoryItems).toEqual([]);
    idle();
  },
);

it("manual repository validation remains usable after StrictMode lifetime setup", async () => {
  await mount(workspace(), true);
  edit();
  const call = start();
  await settle(0);
  await call;
  fireEvent.click(confirm());
  expect(latest.repositoryItems[0].local_path).toBe(CANONICAL);
  noCreation();
});

it("manual repository validation rejects nonempty input without a workspace", async () => {
  await mount(null);
  edit();
  await start();
  act(() => latest.handleConfirmLocalRepository());
  expect(tickets).toHaveLength(0);
  expect(latest.repositoryItems).toEqual([]);
  expect(confirm().disabled).toBe(true);
});
