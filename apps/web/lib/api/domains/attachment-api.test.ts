import { afterEach, describe, expect, it, vi } from "vitest";
import { uploadAttachment } from "./attachment-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

describe("uploadAttachment", () => {
  it("streams a multipart file upload without forcing a JSON content type", async () => {
    const fetchSpy = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            attachment_id: "att-1",
            name: "archive.zip",
            mime_type: "application/zip",
            kind: "resource",
            delivery_mode: "path",
            size_bytes: 104857600,
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const file = new File(["bytes"], "archive.zip", { type: "application/zip" });
    const descriptor = await uploadAttachment(file, {
      workspaceId: "workspace-1",
      kind: "resource",
      deliveryMode: "path",
    });

    expect(descriptor.attachment_id).toBe("att-1");
    const [, init] = fetchSpy.mock.calls[0] as unknown as [string, RequestInit];
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect((init.headers as Record<string, string> | undefined)?.["Content-Type"]).toBeUndefined();
    expect(init.body).toBeInstanceOf(FormData);
    const form = init.body as FormData;
    expect(form.get("workspace_id")).toBe("workspace-1");
    expect(form.get("file")).toBeInstanceOf(File);
  });

  it("preserves the server status when a 100 MiB upload is rejected", async () => {
    global.fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "Attachment exceeds the maximum size" }), {
          status: 413,
          headers: { "Content-Type": "application/json" },
        }),
    ) as typeof global.fetch;

    await expect(
      uploadAttachment(new File(["bytes"], "too-large.bin"), {
        workspaceId: "workspace-1",
        kind: "resource",
        deliveryMode: "path",
      }),
    ).rejects.toMatchObject({ status: 413 });
  });
});
