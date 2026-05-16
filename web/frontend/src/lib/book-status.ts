import type { TFunction } from "../hooks/use-i18n";

export type BookStatus =
  | "draft"
  | "active"
  | "paused"
  | "completed"
  | "archived"
  | "outlining"
  | "dropped";

export interface BookStatusOption {
  readonly value: BookStatus;
  readonly label: string;
}

export function getBookStatusLabel(status: string, t: TFunction): string {
  switch (status) {
    case "draft":
      return t("book.statusDraft");
    case "active":
      return t("book.statusActive");
    case "paused":
      return t("book.statusPaused");
    case "completed":
      return t("book.statusCompleted");
    case "archived":
      return t("book.statusArchived");
    case "outlining":
      return t("book.statusOutlining");
    case "dropped":
      return t("book.statusDropped");
    default:
      return status;
  }
}

export function getBookStatusTone(status: string): string {
  switch (status) {
    case "active":
      return "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    case "paused":
      return "bg-amber-500/10 text-amber-700 dark:text-amber-300";
    case "completed":
      return "bg-sky-500/10 text-sky-700 dark:text-sky-300";
    case "archived":
      return "bg-slate-500/10 text-slate-700 dark:text-slate-300";
    case "dropped":
      return "bg-destructive/10 text-destructive";
    default:
      return "bg-secondary text-muted-foreground";
  }
}

export function buildBookStatusOptions(t: TFunction): ReadonlyArray<BookStatusOption> {
  return [
    { value: "draft", label: t("book.statusDraft") },
    { value: "active", label: t("book.statusActive") },
    { value: "paused", label: t("book.statusPaused") },
    { value: "completed", label: t("book.statusCompleted") },
    { value: "archived", label: t("book.statusArchived") },
  ];
}
