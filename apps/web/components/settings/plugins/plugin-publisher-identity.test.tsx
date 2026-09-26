import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PluginPublisherIdentity, publisherIdentitiesMatch } from "./plugin-publisher-identity";

afterEach(cleanup);

describe("PluginPublisherIdentity", () => {
  it("shows verified publisher, source, and declared author as separate facts", () => {
    render(
      <PluginPublisherIdentity
        identity={{
          status: "verified",
          login: "acme",
          repository: "acme/example",
          official: false,
        }}
        sourceName="Kandev Official"
        author="Example contributors"
      />,
    );

    expect(screen.getByText("Publisher: acme")).toBeTruthy();
    expect(screen.getByText("Verified publisher")).toBeTruthy();
    expect(screen.getByText("Source:")).toBeTruthy();
    expect(screen.getByText("Kandev Official")).toBeTruthy();
    expect(screen.getByText("Declared author:")).toBeTruthy();
    expect(screen.getByText("Example contributors")).toBeTruthy();
  });

  it("does not promote an unverified author to a publisher", () => {
    render(
      <PluginPublisherIdentity
        identity={{ status: "unverified" }}
        provenance={{ origin: "upload", package_id: "example", version: "1.0.0" }}
        author="kandev"
      />,
    );

    expect(screen.getByText("Unverified publisher")).toBeTruthy();
    expect(screen.queryByText("Publisher: kandev")).toBeNull();
    expect(screen.getByText("Uploaded file")).toBeTruthy();
    expect(screen.getByText("kandev")).toBeTruthy();
  });

  it("supports a compact trust row", () => {
    render(
      <PluginPublisherIdentity
        compact
        identity={{ status: "unverified" }}
        author="Example contributors"
      />,
    );

    expect(screen.getByTestId("plugin-publisher-identity").className).toContain("flex flex-wrap");
  });
});

describe("publisherIdentitiesMatch", () => {
  it("matches two unverified projections without inventing an identity", () => {
    expect(publisherIdentitiesMatch({ status: "unverified" }, { status: "unverified" })).toBe(true);
  });

  it("requires all trusted identity fields to remain stable", () => {
    const first = { status: "verified" as const, login: "acme", repository_id: "1" };
    expect(publisherIdentitiesMatch(first, { ...first })).toBe(true);
    expect(publisherIdentitiesMatch(first, { ...first, login: "other" })).toBe(false);
  });
});
