import { getTruthFilesFromResponse } from "./truth-files";

export interface BootstrapSnapshot {
  readonly source: string;
  readonly brief: string;
  readonly currentFocus: string;
  readonly coreConflict: string;
  readonly worldAnchor: string;
  readonly truthFileCount: number;
  readonly openHookCount: number;
  readonly artifacts: ReadonlyArray<BootstrapArtifactSnapshot>;
}

export interface BootstrapArtifactSnapshot {
  readonly key: string;
  readonly title: string;
  readonly jobTitle: string;
  readonly responsibility: string;
  readonly backingFiles: ReadonlyArray<string>;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asArray(value: unknown): ReadonlyArray<unknown> {
  return Array.isArray(value) ? value : [];
}

export function summarizeBootstrapSnapshot(payload: Record<string, unknown> | null | undefined): BootstrapSnapshot | null {
  const truthFileCount = getTruthFilesFromResponse(payload).length;
  const currentState = asRecord(payload?.currentState);
  const foundation = asRecord(currentState?.foundation);
  const storyBible = asRecord(foundation?.storyBible);
  const particleLedger = asRecord(payload?.particleLedger);
  const pendingHooks = Array.isArray(payload?.pendingHooks) ? payload.pendingHooks : [];

  if (!currentState && !truthFileCount) {
    return null;
  }

  const openHookCount = pendingHooks.filter((item) => {
    const row = asRecord(item);
    return !row || asString(row.status) === "" || asString(row.status) === "open";
  }).length;
  const artifacts = asArray(foundation?.artifacts)
    .map((item) => {
      const row = asRecord(item);
      if (!row) {
        return null;
      }
      const key = asString(row.key);
      const title = asString(row.title);
      if (!key || !title) {
        return null;
      }
      return {
        key,
        title,
        jobTitle: asString(row.jobTitle),
        responsibility: asString(row.responsibility),
        backingFiles: asArray(row.backingFiles).map(asString).filter(Boolean),
      } satisfies BootstrapArtifactSnapshot;
    })
    .filter((item): item is BootstrapArtifactSnapshot => item !== null);

  return {
    source: asString(foundation?.source) || "fallback",
    brief: asString(foundation?.brief) || asString(currentState?.authorIntent),
    currentFocus: asString(currentState?.currentFocus),
    coreConflict: asString(storyBible?.coreConflict) || asString(particleLedger?.coreConflict),
    worldAnchor: asString(storyBible?.worldAnchor) || asString(particleLedger?.worldAnchor),
    truthFileCount,
    openHookCount,
    artifacts,
  };
}
