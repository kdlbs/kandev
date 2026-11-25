import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useRouter, usePathname } from "@/lib/routing/client-router";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { SettingsSaveProvider } from "../settings-save-provider";
import { SSHSetupContinuationProvider, useSSHSetupContinuation } from "./setup-continuation";
const editor = "/settings/executors/new/ssh";
const manager = "/settings/executors/ssh-connections";
afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
});
function Probe() {
  const flow = useSSHSetupContinuation();
  const router = useRouter();
  const store = useAppStoreApi();
  return (
    <>
      <output>{JSON.stringify(flow.draft)}</output>
      <button
        onClick={() =>
          flow.begin({
            editor,
            name: "Unsaved",
            connection: "original",
            baseline: "base",
            ownerUserId: "owner",
            ...{ password: "must-not-survive" },
          })
        }
      >
        Begin
      </button>
      <button onClick={() => router.push(manager)}>Manage</button>
      <button onClick={() => router.push(editor)}>Return</button>
      <button onClick={() => router.push("/settings/executors")}>Exit</button>
      <button onClick={() => flow.select("chosen")}>Use</button>
      <button onClick={flow.clear}>Discard</button>
      <button
        onClick={() =>
          store.getState().setAuthState({ mode: "enabled", authenticated: false, user: null })
        }
      >
        Logout
      </button>
      <button
        onClick={() =>
          flow.begin({
            editor: "https://example.invalid",
            name: "Invalid",
            connection: "",
            baseline: "",
            ownerUserId: "",
          })
        }
      >
        Invalid
      </button>
      <button
        onClick={() =>
          store.setState((s) => ({ features: { ...s.features, sshConnectionManager: false } }))
        }
      >
        Disable
      </button>
    </>
  );
}
function Routes() {
  const path = usePathname();
  return (
    <SSHSetupContinuationProvider>
      <SettingsSaveProvider key={path}>
        <Probe />
      </SettingsSaveProvider>
    </SSHSetupContinuationProvider>
  );
}
function setup() {
  window.history.replaceState({}, "", editor);
  return render(
    <StateProvider
      initialState={{ features: { ...defaultFeatureFlags, sshConnectionManager: true } }}
    >
      <Routes />
    </StateProvider>,
  );
}
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
it("keeps only allowed metadata across pathname-keyed save-provider remounts", () => {
  setup();
  click("Begin");
  click("Manage");
  click("Return");
  expect(JSON.parse(screen.getByRole("status").textContent!)).toEqual({
    editor,
    name: "Unsaved",
    connection: "original",
    baseline: "base",
    ownerUserId: "owner",
  });
});
it("changes selection only on explicit use and clears when leaving the flow", () => {
  setup();
  click("Begin");
  click("Manage");
  click("Use");
  click("Return");
  expect(JSON.parse(screen.getByRole("status").textContent!).connection).toBe("chosen");
  click("Exit");
  click("Return");
  expect(screen.getByRole("status").textContent).toBe("null");
});
it.each(["Discard", "Disable", "Logout"])("clears the continuation on %s", (action) => {
  setup();
  click("Begin");
  click(action);
  expect(screen.getByRole("status").textContent).toBe("null");
});
it("does not persist a continuation after the settings shell unmounts", () => {
  const mounted = setup();
  click("Begin");
  act(() => mounted.unmount());
  setup();
  expect(screen.getByRole("status").textContent).toBe("null");
});

it("rejects an external return route", () => {
  setup();
  click("Invalid");
  expect(screen.getByRole("status").textContent).toBe("null");
});
