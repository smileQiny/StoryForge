import { fetchJson, useApi } from "../hooks/use-api";
import { useState } from "react";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import { useColors } from "../hooks/use-colors";
import { getTruthFileContent, getTruthFilesFromResponse } from "../lib/truth-files";
import { Pencil, Save, X } from "lucide-react";

interface Nav {
  toBook: (id: string) => void;
  toDashboard: () => void;
}

export function TruthFiles({ bookId, nav, theme, t }: { bookId: string; nav: Nav; theme: Theme; t: TFunction }) {
  const c = useColors(theme);
  const { data, refetch: refetchList } = useApi<Record<string, unknown> | { files: ReadonlyArray<{ name: string; size: number; preview: string }> }>(`/books/${bookId}/truth`);
  const [selected, setSelected] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [editText, setEditText] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);
  const { data: fileData, refetch: refetchFile } = useApi<{ file: string; content: string | null } | string | Record<string, unknown> | null>(
    selected ? `/books/${bookId}/truth/${selected}` : "",
  );
  const files = getTruthFilesFromResponse(data);
  const normalizedFileData = getTruthFileContent(fileData, selected ?? "");

  const startEdit = () => {
    setEditText(normalizedFileData.content ?? "");
    setEditMode(true);
  };

  const cancelEdit = () => {
    setEditMode(false);
  };

  const handleSaveEdit = async () => {
    if (!selected) return;
    setSavingEdit(true);
    try {
      JSON.parse(editText);
      await fetchJson(`/books/${bookId}/truth/${selected}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: editText,
      });
      setEditMode(false);
      refetchFile();
      refetchList();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save");
    } finally {
      setSavingEdit(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <button onClick={nav.toDashboard} className={c.link}>{t("bread.books")}</button>
        <span className="text-border">/</span>
        <button onClick={() => nav.toBook(bookId)} className={c.link}>{bookId}</button>
        <span className="text-border">/</span>
        <span className="text-foreground">{t("truth.title")}</span>
      </div>

      <h1 className="font-serif text-3xl">{t("truth.title")}</h1>

      <div className="grid grid-cols-[240px_1fr] gap-6">
        {/* File list */}
        <div className={`border ${c.cardStatic} rounded-lg overflow-hidden`}>
          {files.map((f) => (
            <button
              key={f.name}
              onClick={() => { setSelected(f.name); setEditMode(false); }}
              className={`w-full text-left px-3 py-2.5 text-sm border-b border-border/40 transition-colors ${
                selected === f.name
                  ? "bg-primary/10 text-primary"
                  : "hover:bg-muted/30 text-muted-foreground"
              }`}
            >
              <div className="font-mono text-sm truncate">{f.name}</div>
              <div className="text-xs text-muted-foreground mt-0.5">{f.size.toLocaleString()} {t("truth.chars")}</div>
            </button>
          ))}
          {files.length === 0 && (
            <div className="px-3 py-4 text-sm text-muted-foreground text-center">{t("truth.empty")}</div>
          )}
        </div>

        {/* Content viewer */}
        <div className={`border ${c.cardStatic} rounded-lg p-5 min-h-[400px] flex flex-col`}>
          {selected && normalizedFileData.content != null ? (
            <>
              <div className="flex items-center justify-end gap-2 mb-3">
                {editMode ? (
                  <>
                    <button
                      onClick={cancelEdit}
                      className={`inline-flex items-center gap-1 px-3 py-1.5 text-xs rounded-md ${c.btnSecondary}`}
                    >
                      <X size={14} />
                      Cancel
                    </button>
                    <button
                      onClick={handleSaveEdit}
                      disabled={savingEdit}
                      className={`inline-flex items-center gap-1 px-3 py-1.5 text-xs rounded-md ${c.btnPrimary} disabled:opacity-50`}
                    >
                      <Save size={14} />
                      {savingEdit ? t("truth.saving") : t("truth.save")}
                    </button>
                  </>
                ) : (
                  <button
                    onClick={startEdit}
                    className={`inline-flex items-center gap-1 px-3 py-1.5 text-xs rounded-md ${c.btnSecondary}`}
                  >
                    <Pencil size={14} />
                    Edit
                  </button>
                )}
              </div>
              {editMode ? (
                <textarea
                  value={editText}
                  onChange={(e) => setEditText(e.target.value)}
                  className={`${c.input} flex-1 rounded-md p-3 text-sm font-mono leading-relaxed resize-none min-h-[360px]`}
                />
              ) : (
                <pre className="text-sm leading-relaxed whitespace-pre-wrap font-mono text-foreground/80">{normalizedFileData.content}</pre>
              )}
            </>
          ) : selected && normalizedFileData.content === null ? (
            <div className="text-muted-foreground text-sm">{t("truth.notFound")}</div>
          ) : (
            <div className="text-muted-foreground/50 text-sm italic">{t("truth.selectFile")}</div>
          )}
        </div>
      </div>
    </div>
  );
}
