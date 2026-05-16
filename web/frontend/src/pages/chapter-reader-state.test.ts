import { describe, expect, it } from "vitest";
import { summarizeChapterAnalysis } from "../lib/chapter-analysis";

describe("summarizeChapterAnalysis", () => {
  it("reports counts from facts and delta collections", () => {
    expect(summarizeChapterAnalysis({
      facts: [{}, {}, {}],
      delta: {
        newHookCandidates: [{}, {}],
        subplotOps: [{}],
        emotionalArcOps: [{}],
        characterMatrixOps: [{}, {}, {}],
      },
    })).toEqual({
      factCount: 3,
      hookCount: 2,
      subplotCount: 1,
      emotionalArcCount: 1,
      matrixCount: 3,
    });
  });

  it("falls back to zero counts when the payload is sparse", () => {
    expect(summarizeChapterAnalysis({})).toEqual({
      factCount: 0,
      hookCount: 0,
      subplotCount: 0,
      emotionalArcCount: 0,
      matrixCount: 0,
    });
  });
});
