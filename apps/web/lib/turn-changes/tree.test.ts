import { expect, it } from "vitest";
import {
  buildTurnChangeTree,
  summarizeTurnChangeTreeFolder,
  turnChangeRepositoryOptionName,
} from "./tree";

const readyAvailability = "ready" as const;
const checkoutA = "checkout-a";
const checkoutB = "checkout-b";

it("keeps same relative paths separate by actual checkout and preserves root files", () => {
  const tree = buildTurnChangeTree(
    [
      {
        id: "repo-a",
        checkout_id: checkoutA,
        display_name: "app",
        availability: readyAvailability,
        enumeration_complete: true,
        comparison_complete: true,
        content_complete: true,
      },
      {
        id: "repo-b",
        checkout_id: checkoutB,
        display_name: "library",
        availability: readyAvailability,
        enumeration_complete: true,
        comparison_complete: true,
        content_complete: true,
      },
    ],
    {
      "repo-a": [
        {
          id: "a1",
          repository_change_id: "repo-a",
          checkout_id: checkoutA,
          path: "src/shared.ts",
          kind: "modified",
          added_lines: 3,
          deleted_lines: 1,
          content_availability: readyAvailability,
        },
        {
          id: "a2",
          repository_change_id: "repo-a",
          checkout_id: checkoutA,
          path: "logo.png",
          kind: "added",
          binary: true,
          content_availability: readyAvailability,
        },
      ],
      "repo-b": [
        {
          id: "b1",
          repository_change_id: "repo-b",
          checkout_id: checkoutB,
          path: "src/shared.ts",
          kind: "renamed",
          old_path: "src/old.ts",
          content_availability: readyAvailability,
        },
      ],
    },
    "Repository",
  );

  expect(tree.map((repository) => repository.id)).toEqual(["repo-a", "repo-b"]);
  expect(tree[0].folders[0].files[0].file.id).toBe("a1");
  expect(tree[1].folders[0].files[0].file.id).toBe("b1");
  expect(tree[0].files[0].file.path).toBe("logo.png");

  expect(summarizeTurnChangeTreeFolder(tree[0].folders[0]!)).toEqual({
    loadedFiles: 1,
    addedLines: 3,
    deletedLines: 1,
    unknownCountFiles: 0,
  });
});

it("aggregates nested loaded folders and keeps unknown file counts explicit", () => {
  const tree = buildTurnChangeTree(
    [
      {
        id: "repo",
        checkout_id: "checkout",
        availability: "ready",
        enumeration_complete: false,
        comparison_complete: false,
        content_complete: false,
      },
    ],
    {
      repo: [
        {
          id: "a",
          repository_change_id: "repo",
          checkout_id: "checkout",
          path: "src/a.ts",
          kind: "added",
          added_lines: 4,
          deleted_lines: 0,
          content_availability: readyAvailability,
        },
        {
          id: "b",
          repository_change_id: "repo",
          checkout_id: "checkout",
          path: "src/lib/b.ts",
          kind: "mode_changed",
          content_availability: readyAvailability,
        },
      ],
    },
    "Repository",
  );

  expect(summarizeTurnChangeTreeFolder(tree[0]!.folders[0]!)).toEqual({
    loadedFiles: 2,
    addedLines: 4,
    deletedLines: 0,
    unknownCountFiles: 1,
  });
});

it("disambiguates identical file paths by actual checkout in the selector label", () => {
  const repositories = [
    {
      id: "repo-a",
      checkout_id: checkoutA,
      display_name: "app",
      availability: readyAvailability,
      enumeration_complete: true,
      comparison_complete: true,
      content_complete: true,
    },
    {
      id: "repo-b",
      checkout_id: checkoutB,
      display_name: "app",
      availability: readyAvailability,
      enumeration_complete: true,
      comparison_complete: true,
      content_complete: true,
    },
  ];
  expect(turnChangeRepositoryOptionName(repositories, "repo-a", checkoutA)).toBe(
    "app (checkout-a)",
  );
  expect(turnChangeRepositoryOptionName(repositories, "repo-b", checkoutB)).toBe(
    "app (checkout-b)",
  );
});
