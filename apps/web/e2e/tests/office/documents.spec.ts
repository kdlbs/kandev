import { test, expect } from "../../fixtures/office-fixture";

test.describe("Documents", () => {
  test("create and retrieve document", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document Create Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "spec", {
      type: "spec",
      title: "Feature Spec",
      content: "# Spec\n\nThis is a test spec.",
    });

    const docResp = await officeApi.getDocument(taskId, "spec");
    const doc = (docResp as { document: { key: string; title: string; content: string } }).document;

    expect(doc.key).toBe("spec");
    expect(doc.title).toBe("Feature Spec");
    expect(doc.content).toContain("test spec");
  });

  test("update document increments revision number", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document Revisions Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "plan", {
      type: "plan",
      title: "Plan v1",
      content: "Initial plan content.",
      author_kind: "user",
      author_name: "User A",
    });

    await officeApi.createOrUpdateDocument(taskId, "plan", {
      type: "plan",
      title: "Plan v2",
      content: "Updated plan content.",
      author_kind: "user",
      author_name: "User B",
    });

    const revisionsResp = await officeApi.listRevisions(taskId, "plan");
    const revisions = (
      revisionsResp as { revisions: Array<{ revision_number: number; content: string }> }
    ).revisions;

    // Two creates/updates should produce at least 2 revisions
    expect(revisions.length).toBeGreaterThanOrEqual(2);
    const numbers = revisions.map((r) => r.revision_number);
    expect(Math.max(...numbers)).toBeGreaterThan(Math.min(...numbers));
  });

  test("list documents returns all documents for task", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document List Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "notes", {
      type: "notes",
      title: "Notes",
      content: "Some notes.",
    });
    await officeApi.createOrUpdateDocument(taskId, "design", {
      type: "design",
      title: "Design",
      content: "Design doc content.",
    });

    const listResp = await officeApi.listDocuments(taskId);
    const docs = (listResp as { documents: Array<{ key: string }> }).documents;
    const keys = docs.map((d) => d.key);

    expect(keys).toContain("notes");
    expect(keys).toContain("design");
  });

  test("delete document removes it", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document Delete Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "to-delete", {
      type: "misc",
      title: "Temp Doc",
      content: "Will be deleted.",
    });

    // Verify it exists before deletion
    const before = await officeApi.listDocuments(taskId);
    const beforeDocs = (before as { documents: Array<{ key: string }> }).documents;
    expect(beforeDocs.some((d) => d.key === "to-delete")).toBe(true);

    await officeApi.deleteDocument(taskId, "to-delete");

    // Should be gone
    const after = await officeApi.listDocuments(taskId);
    const afterDocs = (after as { documents: Array<{ key: string }> }).documents;
    expect(afterDocs.some((d) => d.key === "to-delete")).toBe(false);
  });

  test("list revisions shows revision history", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document History Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "history-doc", {
      type: "notes",
      title: "Rev 1",
      content: "First version.",
      author_kind: "user",
      author_name: "Author A",
    });
    await officeApi.createOrUpdateDocument(taskId, "history-doc", {
      type: "notes",
      title: "Rev 2",
      content: "Second version.",
      author_kind: "user",
      author_name: "Author B",
    });
    await officeApi.createOrUpdateDocument(taskId, "history-doc", {
      type: "notes",
      title: "Rev 3",
      content: "Third version.",
      author_kind: "user",
      author_name: "Author C",
    });

    const revisionsResp = await officeApi.listRevisions(taskId, "history-doc");
    const revisions = (
      revisionsResp as { revisions: Array<{ revision_number: number; title: string }> }
    ).revisions;

    expect(revisions.length).toBeGreaterThanOrEqual(3);
  });

  test("revert to prior revision creates new revision with old content", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Document Revert Task", {
      workflow_id: officeSeed.workflowId,
    });
    const taskId = task.id;

    await officeApi.createOrUpdateDocument(taskId, "revert-doc", {
      type: "notes",
      title: "Original",
      content: "Original content.",
    });
    await officeApi.createOrUpdateDocument(taskId, "revert-doc", {
      type: "notes",
      title: "Modified",
      content: "Modified content.",
    });

    // List revisions to get the first revision ID
    const revisionsResp = await officeApi.listRevisions(taskId, "revert-doc");
    const revisions = (
      revisionsResp as {
        revisions: Array<{ id: string; revision_number: number; content: string }>;
      }
    ).revisions;

    // Find the earliest revision (lowest revision_number)
    const sorted = [...revisions].sort((a, b) => a.revision_number - b.revision_number);
    const firstRevision = sorted[0];
    expect(firstRevision).toBeDefined();

    const revertResp = await officeApi.revertDocument(taskId, "revert-doc", firstRevision.id);
    const reverted = (revertResp as { revision: { content: string; revision_number: number } })
      .revision;

    // Reverted revision should have the original content
    expect(reverted.content).toBe(firstRevision.content);
    // And a higher revision number than before
    expect(reverted.revision_number).toBeGreaterThan(firstRevision.revision_number);
  });
});
