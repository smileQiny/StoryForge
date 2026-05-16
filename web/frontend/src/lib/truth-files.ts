export interface TruthFile {
  readonly name: string;
  readonly size: number;
  readonly preview: string;
}

type TruthResponse =
  | { readonly files?: ReadonlyArray<TruthFile> | null }
  | Record<string, unknown>
  | null
  | undefined;

type TruthFileContentResponse =
  | { readonly file?: string; readonly content?: string | null }
  | string
  | Record<string, unknown>
  | null
  | undefined;

const TRUTH_FILE_KEYS = [
  { key: "currentState", name: "current_state.json" },
  { key: "particleLedger", name: "particle_ledger.json" },
  { key: "pendingHooks", name: "pending_hooks.json" },
  { key: "chapterSummaries", name: "chapter_summaries.json" },
  { key: "subplotBoard", name: "subplot_board.json" },
  { key: "emotionalArcs", name: "emotional_arcs.json" },
  { key: "characterMatrix", name: "character_matrix.json" },
] as const;

function serializeTruthValue(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

export function getTruthFilesFromResponse(payload: TruthResponse): ReadonlyArray<TruthFile> {
  if (payload && "files" in payload && Array.isArray(payload.files)) {
    return payload.files;
  }

  if (!payload || typeof payload !== "object") {
    return [];
  }

  return TRUTH_FILE_KEYS.flatMap(({ key, name }) => {
    const value = payload[key];
    if (value == null) return [];

    const content = serializeTruthValue(value);
    return [{
      name,
      size: content.length,
      preview: content.split("\n")[0]?.slice(0, 120) ?? "",
    }];
  });
}

export function getTruthFileContent(
  payload: TruthFileContentResponse,
  selectedFile: string,
): { readonly file: string; readonly content: string | null } {
  if (payload && typeof payload === "object" && "content" in payload) {
    return {
      file: typeof payload.file === "string" ? payload.file : selectedFile,
      content: typeof payload.content === "string" || payload.content === null ? payload.content : serializeTruthValue(payload.content),
    };
  }

  if (payload == null) {
    return { file: selectedFile, content: null };
  }

  return {
    file: selectedFile,
    content: serializeTruthValue(payload),
  };
}
