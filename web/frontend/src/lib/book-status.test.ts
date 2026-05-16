import { describe, expect, it } from "vitest";
import { buildBookStatusOptions, getBookStatusLabel } from "./book-status";

const t = (key: string): string => {
  const labels: Record<string, string> = {
    "book.statusDraft": "草稿中",
    "book.statusActive": "进行中",
    "book.statusPaused": "已暂停",
    "book.statusCompleted": "已完成",
    "book.statusArchived": "已归档",
    "book.statusOutlining": "大纲中",
    "book.statusDropped": "已放弃",
  };
  return labels[key] ?? key;
};

describe("getBookStatusLabel", () => {
  it("maps draft books to a draft label instead of dropped", () => {
    expect(getBookStatusLabel("draft", t as never)).toBe("草稿中");
  });

  it("maps archived books to an archived label", () => {
    expect(getBookStatusLabel("archived", t as never)).toBe("已归档");
  });
});

describe("buildBookStatusOptions", () => {
  it("offers the backend-supported book statuses in settings", () => {
    expect(buildBookStatusOptions(t as never).map((option) => option.value)).toEqual([
      "draft",
      "active",
      "paused",
      "completed",
      "archived",
    ]);
  });
});
