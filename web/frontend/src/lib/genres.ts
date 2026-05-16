export interface GenreInfo {
  readonly id: string;
  readonly name: string;
  readonly source: "project" | "builtin";
  readonly language: "zh" | "en";
}

type RawGenreInfo = Partial<GenreInfo> & {
  readonly rules?: ReadonlyArray<string> | null;
  readonly fatigueWords?: ReadonlyArray<string> | null;
  readonly numericalSystem?: boolean;
  readonly powerScaling?: boolean;
  readonly eraResearch?: boolean;
  readonly auditDimensions?: ReadonlyArray<string | number> | null;
};

type GenresResponse =
  | ReadonlyArray<RawGenreInfo>
  | { readonly genres?: ReadonlyArray<RawGenreInfo> | null }
  | null
  | undefined;

interface RawGenreDetail {
  readonly profile?: {
    readonly name?: string;
    readonly id?: string;
    readonly language?: string;
    readonly chapterTypes?: ReadonlyArray<string> | null;
    readonly fatigueWords?: ReadonlyArray<string> | null;
    readonly numericalSystem?: boolean;
    readonly powerScaling?: boolean;
    readonly eraResearch?: boolean;
    readonly pacingRule?: string;
    readonly auditDimensions?: ReadonlyArray<string | number> | null;
  } | null;
  readonly body?: string | null;
  readonly id?: string;
  readonly name?: string;
  readonly language?: string;
  readonly chapterTypes?: ReadonlyArray<string> | null;
  readonly fatigueWords?: ReadonlyArray<string> | null;
  readonly numericalSystem?: boolean;
  readonly powerScaling?: boolean;
  readonly eraResearch?: boolean;
  readonly pacingRule?: string;
  readonly rules?: ReadonlyArray<string> | string | null;
  readonly auditDimensions?: ReadonlyArray<string | number> | null;
}

export interface GenreDetail {
  readonly profile: {
    readonly name: string;
    readonly id: string;
    readonly language: string;
    readonly chapterTypes: ReadonlyArray<string>;
    readonly fatigueWords: ReadonlyArray<string>;
    readonly numericalSystem: boolean;
    readonly powerScaling: boolean;
    readonly eraResearch: boolean;
    readonly pacingRule: string;
    readonly auditDimensions: ReadonlyArray<string | number>;
  };
  readonly body: string;
}

export function getGenresFromResponse(payload: GenresResponse): ReadonlyArray<GenreInfo> {
  const genres = Array.isArray(payload) ? payload : payload?.genres ?? [];
  return genres
    .map(normalizeGenreInfo)
    .filter((genre) => Boolean(genre.id));
}

export function normalizeGenreInfo(payload: RawGenreInfo): GenreInfo {
  return {
    id: payload.id ?? "",
    name: payload.name ?? payload.id ?? "",
    source: payload.source === "project" ? "project" : "builtin",
    language: payload.language === "en" ? "en" : "zh",
  };
}

export function buildGenreDetailPath(genreId: string, language?: string): string {
  if (!genreId) return "";

  const params = new URLSearchParams();
  if (language) {
    params.set("language", language);
  }

  const query = params.toString();
  return query ? `/genres/${genreId}?${query}` : `/genres/${genreId}`;
}

export function normalizeGenreDetail(payload: RawGenreDetail | null | undefined): GenreDetail | null {
  if (!payload) return null;

  if (payload.profile) {
    return {
      profile: {
        id: payload.profile.id ?? "",
        name: payload.profile.name ?? payload.profile.id ?? "",
        language: payload.profile.language ?? "zh",
        chapterTypes: payload.profile.chapterTypes ?? [],
        fatigueWords: payload.profile.fatigueWords ?? [],
        numericalSystem: Boolean(payload.profile.numericalSystem),
        powerScaling: Boolean(payload.profile.powerScaling),
        eraResearch: Boolean(payload.profile.eraResearch),
        pacingRule: payload.profile.pacingRule ?? "",
        auditDimensions: payload.profile.auditDimensions ?? [],
      },
      body: payload.body ?? "",
    };
  }

  const rules =
    Array.isArray(payload.rules) ? payload.rules.join("\n")
      : typeof payload.rules === "string" ? payload.rules
      : payload.body ?? "";

  return {
    profile: {
      id: payload.id ?? "",
      name: payload.name ?? payload.id ?? "",
      language: payload.language ?? "zh",
      chapterTypes: payload.chapterTypes ?? [],
      fatigueWords: payload.fatigueWords ?? [],
      numericalSystem: Boolean(payload.numericalSystem),
      powerScaling: Boolean(payload.powerScaling),
      eraResearch: Boolean(payload.eraResearch),
      pacingRule: payload.pacingRule ?? "",
      auditDimensions: payload.auditDimensions ?? [],
    },
    body: rules,
  };
}
