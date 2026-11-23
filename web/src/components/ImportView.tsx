import { useRef, useState, type DragEvent, type FormEvent } from "react";
import { FileUp } from "lucide-react";
import { api, ApiError } from "../api";
import type { ImportResult } from "../types";

interface ImportViewProps {
  onImported: () => Promise<void>;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

export function ImportView({ onImported, onNotify }: ImportViewProps) {
  const [format, setFormat] = useState<"auto" | "netscape" | "karakeep">("auto");
  const [file, setFile] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  function handleDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setDragOver(false);
    const dropped = e.dataTransfer.files?.[0];
    if (dropped) {
      setFile(dropped);
      setResult(null);
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!file) return;
    setUploading(true);
    setResult(null);
    try {
      const res = await api.importFile(file, format);
      setResult(res);
      await onImported();
      onNotify(`Imported ${res.imported} bookmarks (${res.skipped} skipped)`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Import failed", "error");
    } finally {
      setUploading(false);
    }
  }

  return (
    <section className="panel" aria-labelledby="import-heading">
      <div>
        <h1 id="import-heading" style={{ margin: 0, fontSize: 19, fontWeight: 700 }}>
          Import Bookmarks
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 14, color: "var(--fg-muted)" }}>
          Upload a browser Netscape HTML bookmark file or a Karakeep JSON export. Duplicate URLs are skipped automatically.
        </p>
      </div>

      <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div className="field" style={{ maxWidth: 280 }}>
          <label className="field-label" htmlFor="import-format">
            File format
          </label>
          <select
            id="import-format"
            className="select"
            value={format}
            onChange={(e) => setFormat(e.target.value as "auto" | "netscape" | "karakeep")}
          >
            <option value="auto">Auto-detect (.html or .json)</option>
            <option value="netscape">Netscape HTML (Chrome / Firefox / Safari)</option>
            <option value="karakeep">Karakeep JSON export</option>
          </select>
        </div>

        <div
          className={`dropzone ${dragOver ? "is-dragover" : ""}`}
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
        >
          <FileUp size={24} aria-hidden="true" style={{ color: "var(--accent-text)" }} />
          <div style={{ fontSize: 14, fontWeight: 500 }}>
            {file ? file.name : "Drop a bookmark export file here, or choose from disk"}
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept=".html,.htm,.json,text/html,application/json"
            className="sr-only"
            id="import-file-input"
            onChange={(e) => {
              const picked = e.target.files?.[0] ?? null;
              setFile(picked);
              setResult(null);
            }}
          />
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => fileInputRef.current?.click()}
          >
            Choose file
          </button>
        </div>

        <div>
          <button type="submit" className="btn btn-primary" disabled={!file || uploading}>
            {uploading ? "Importing..." : "Start import"}
          </button>
        </div>
      </form>

      {result && (
        <div
          style={{
            padding: 14,
            borderRadius: "var(--radius-md)",
            backgroundColor: "var(--surface-muted)",
            border: "1px solid var(--border)",
            display: "flex",
            flexWrap: "wrap",
            gap: 18,
            fontSize: 14,
          }}
        >
          <div>
            <strong>Format:</strong> {result.format}
          </div>
          <div className="tabular-nums">
            <strong>Imported:</strong> {result.imported}
          </div>
          <div className="tabular-nums">
            <strong>Skipped (duplicates):</strong> {result.skipped}
          </div>
          <div className="tabular-nums">
            <strong>Errors:</strong> {result.errors}
          </div>
        </div>
      )}
    </section>
  );
}
