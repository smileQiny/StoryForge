export interface ChapterAnalyzeDelta {
  readonly newHookCandidates?: ReadonlyArray<unknown>;
  readonly subplotOps?: ReadonlyArray<unknown>;
  readonly emotionalArcOps?: ReadonlyArray<unknown>;
  readonly characterMatrixOps?: ReadonlyArray<unknown>;
}

export interface ChapterAnalyzeResult {
  readonly bookId?: string;
  readonly chapter?: number;
  readonly chapterTitle?: string;
  readonly facts?: ReadonlyArray<unknown>;
  readonly delta?: ChapterAnalyzeDelta;
  readonly currentState?: unknown;
  readonly nextState?: unknown;
  readonly previousSummary?: string;
}

export interface ChapterAnalysisSummary {
  readonly factCount: number;
  readonly hookCount: number;
  readonly subplotCount: number;
  readonly emotionalArcCount: number;
  readonly matrixCount: number;
}

function countItems(value: unknown): number {
  return Array.isArray(value) ? value.length : 0;
}

export function summarizeChapterAnalysis(result?: Partial<ChapterAnalyzeResult> | null): ChapterAnalysisSummary {
  return {
    factCount: countItems(result?.facts),
    hookCount: countItems(result?.delta?.newHookCandidates),
    subplotCount: countItems(result?.delta?.subplotOps),
    emotionalArcCount: countItems(result?.delta?.emotionalArcOps),
    matrixCount: countItems(result?.delta?.characterMatrixOps),
  };
}
