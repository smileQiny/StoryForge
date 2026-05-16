import { describe, expect, it } from "vitest";
import { summarizeBootstrapSnapshot } from "./book-bootstrap";

describe("summarizeBootstrapSnapshot", () => {
  it("extracts documented foundation artifact metadata from aggregated truth payloads", () => {
    const snapshot = summarizeBootstrapSnapshot({
      currentState: {
        foundation: {
          source: "llm",
          brief: "brief",
          storyBible: { coreConflict: "conflict", worldAnchor: "anchor" },
          artifacts: [
            {
              key: "storyBible",
              title: "基础世界观",
              jobTitle: "世界设定架构师",
              responsibility: "定义这本书的世界底座",
            },
            {
              key: "initialHooks",
              title: "初始 Hooks",
              jobTitle: "伏笔设计师",
              responsibility: "设计初始悬念、伏笔和预期回收节奏",
            },
          ],
        },
        currentFocus: "open the story",
      },
      pendingHooks: [{ status: "open" }],
    });

    expect(snapshot).not.toBeNull();
    expect(snapshot?.artifacts).toEqual([
      {
        key: "storyBible",
        title: "基础世界观",
        jobTitle: "世界设定架构师",
        responsibility: "定义这本书的世界底座",
        backingFiles: [],
      },
      {
        key: "initialHooks",
        title: "初始 Hooks",
        jobTitle: "伏笔设计师",
        responsibility: "设计初始悬念、伏笔和预期回收节奏",
        backingFiles: [],
      },
    ]);
  });
});
