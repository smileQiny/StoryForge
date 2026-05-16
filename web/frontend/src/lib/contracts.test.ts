import { describe, expect, it } from "vitest";
import { getBooksFromResponse } from "./books";
import { getTruthFileContent, getTruthFilesFromResponse } from "./truth-files";
import { getGenresFromResponse, normalizeGenreDetail } from "./genres";

describe("books contract", () => {
  it("accepts both wrapped and raw books responses", () => {
    expect(getBooksFromResponse([{ id: "a" }])).toEqual([{ id: "a" }]);
    expect(getBooksFromResponse({ books: [{ id: "b" }] })).toEqual([{ id: "b" }]);
  });
});

describe("truth contract", () => {
  it("derives truth file entries from aggregated truth payloads", () => {
    const files = getTruthFilesFromResponse({
      currentState: { hero: "awake" },
      chapterSummaries: [{ chapter: 1, summary: "intro" }],
    });
    expect(files.map((file) => file.name)).toEqual([
      "current_state.json",
      "chapter_summaries.json",
    ]);
  });

  it("normalizes raw truth file payloads and wrapped content payloads", () => {
    expect(getTruthFileContent({ file: "current_state.json", content: { hero: "awake" } }, "current_state.json")).toEqual({
      file: "current_state.json",
      content: JSON.stringify({ hero: "awake" }, null, 2),
    });
    expect(getTruthFileContent({ hero: "awake" }, "current_state.json")).toEqual({
      file: "current_state.json",
      content: JSON.stringify({ hero: "awake" }, null, 2),
    });
  });
});

describe("genre contract", () => {
  it("accepts both wrapped and raw genre lists", () => {
    expect(getGenresFromResponse([{ id: "xuanhuan", name: "玄幻" }])[0]?.id).toBe("xuanhuan");
    expect(getGenresFromResponse({ genres: [{ id: "mystery", name: "Mystery" }] })[0]?.id).toBe("mystery");
  });

  it("normalizes both modern and legacy genre detail payloads", () => {
    expect(normalizeGenreDetail({
      profile: {
        id: "xuanhuan",
        name: "玄幻",
        chapterTypes: ["升级"],
      },
      body: "rules",
    })?.profile.id).toBe("xuanhuan");

    expect(normalizeGenreDetail({
      id: "mystery",
      name: "Mystery",
      rules: ["keep suspense"],
    })?.body).toBe("keep suspense");
  });
});
