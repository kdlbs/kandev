import { act, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  acceptedSecret,
  existingSecret,
  acceptThroughSave,
  expectMetadataRow,
  finishSettingsRequests,
  holdGlobalRequests,
  mountGlobalSettings,
} from "./secrets-settings.initial-failure.test-helpers";

afterEach(finishSettingsRequests);

describe("Global initial failure through real Settings Save", () => {
  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.5
  it("keeps accepted Global metadata and its row after older initial GET500", async () => {
    const http = holdGlobalRequests();
    const { store } = mountGlobalSettings();
    expect(store.getState().secrets).toMatchObject({ loaded: false, loading: true });
    await acceptThroughSave(store, http);
    const current = store.getState().secrets.items;
    expect(current).toEqual([acceptedSecret]);
    expect(store.getState().secrets).toMatchObject({ loaded: false, loading: true });
    await act(async () => http.release(500, { error: "Synthetic list server failure" }));
    expectMetadataRow(acceptedSecret);
    expect(store.getState().secrets.items).toBe(current);
    expect(store.getState().secrets).toEqual({ items: current, loaded: true, loading: false });
    expect(http.requests.filter((request) => request.method === "GET")).toHaveLength(1);
    expect(http.unexpected).toEqual([]);
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.3
  it("renders uncontested initial GET200 metadata", async () => {
    const http = holdGlobalRequests();
    const { store } = mountGlobalSettings();
    await act(async () => http.release(200, [existingSecret]));
    expectMetadataRow(existingSecret);
    expect(store.getState().secrets).toEqual({
      items: [existingSecret],
      loaded: true,
      loading: false,
    });
    expect(http.requests).toEqual([{ method: "GET", path: "/api/v1/secrets", body: undefined }]);
    expect(http.unexpected).toEqual([]);
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.3
  it("creates on an already-loaded Global list without initial GET", async () => {
    const http = holdGlobalRequests();
    const { store } = mountGlobalSettings([existingSecret], true);
    await acceptThroughSave(store, http);
    expectMetadataRow(existingSecret);
    expectMetadataRow(acceptedSecret);
    expect(store.getState().secrets).toEqual({
      items: [existingSecret, acceptedSecret],
      loaded: true,
      loading: false,
    });
    expect(http.requests.filter((request) => request.method === "GET")).toEqual([]);
    expect(http.unexpected).toEqual([]);
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.6
  it("settles uncontested initial GET500 empty", async () => {
    const http = holdGlobalRequests();
    const { store } = mountGlobalSettings();
    await act(async () => http.release(500, { error: "Synthetic list server failure" }));
    expect(store.getState().secrets).toEqual({ items: [], loaded: true, loading: false });
    expect(http.requests).toEqual([{ method: "GET", path: "/api/v1/secrets", body: undefined }]);
    expect(http.unexpected).toEqual([]);
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.6
  it("clears accepted Global metadata after older initial GET403", async () => {
    const http = holdGlobalRequests();
    const { store } = mountGlobalSettings();
    await acceptThroughSave(store, http);
    expect(store.getState().secrets.items).toEqual([acceptedSecret]);
    await act(async () => http.release(403, { error: "Synthetic access denial" }));
    expect(store.getState().secrets).toEqual({ items: [], loaded: true, loading: false });
    expect(screen.queryByTestId(`secret-row-${acceptedSecret.id}`)).toBeNull();
    expect(http.requests.filter((request) => request.method === "GET")).toHaveLength(1);
    expect(http.unexpected).toEqual([]);
  });
});
