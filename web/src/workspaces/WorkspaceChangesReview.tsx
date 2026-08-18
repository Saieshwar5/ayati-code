import { useEffect, useMemo, useState } from "react";
import type { Changes } from "../api/contracts";

interface ChangedFile {
  path: string;
  status: string;
  patch: string;
  additions: number;
  deletions: number;
}

interface WorkspaceChangesReviewProps {
  changes: Changes;
  loading: boolean;
  error?: string;
  compact?: boolean;
  embedded?: boolean;
  onFileOpen?: () => void;
  onRefresh: () => void;
  onPublish: () => void;
}

export function WorkspaceChangesReview(props: WorkspaceChangesReviewProps) {
  const [query, setQuery] = useState("");
  const files = useMemo(() => parseChanges(props.changes), [props.changes]);
  const visible = files.filter((file) => file.path.toLowerCase().includes(query.trim().toLowerCase()));
  const [selectedPath, setSelectedPath] = useState("");
  const selected = visible.find((file) => file.path === selectedPath) || visible[0];
  const additions = files.reduce((total, file) => total + file.additions, 0);
  const deletions = files.reduce((total, file) => total + file.deletions, 0);

  useEffect(() => {
    if (!files.some((file) => file.path === selectedPath)) setSelectedPath(files[0]?.path || "");
  }, [files, selectedPath]);

  return (
    <section className={`changes-review${props.compact ? " compact-review" : ""}${props.embedded ? " embedded" : ""}`} aria-label="Workspace changes">
      <header className="changes-review-heading">
        <div>
          {!props.embedded && <p className="eyebrow">Review</p>}
          <h2>{files.length ? `${files.length} changed ${files.length === 1 ? "file" : "files"}` : "Workspace changes"}</h2>
          <p>{files.length ? <><strong>+{additions}</strong> <span>−{deletions}</span></> : "Review the working tree before publishing."}</p>
        </div>
        <div>
          <button className="quiet compact" type="button" disabled={props.loading} onClick={props.onRefresh}>{props.loading ? "Refreshing…" : "Refresh"}</button>
          {!props.embedded && <button className="primary compact" type="button" disabled={!files.length || Boolean(props.error)} onClick={props.onPublish}>Publish…</button>}
        </div>
      </header>
      {props.error ? (
        <div className="changes-clean" role="alert"><strong>Unable to load changes</strong><p>{props.error}</p></div>
      ) : files.length ? (
        <div className="changes-review-body">
          <aside className="changed-files" aria-label="Changed files">
            <label><span className="sr-only">Filter changed files</span><input type="search" placeholder="Filter files" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
            <div>
              {visible.map((file) => (
                <button className={selected?.path === file.path ? "selected" : ""} type="button" key={file.path} onClick={() => { setSelectedPath(file.path); props.onFileOpen?.(); }}>
                  <span>{file.status}</span><strong>{file.path}</strong><small>+{file.additions} −{file.deletions}</small>
                </button>
              ))}
              {!visible.length && <p>No matching files.</p>}
            </div>
          </aside>
          <section className="file-diff" aria-label={selected ? `Diff for ${selected.path}` : "File diff"}>
            {selected && <header><strong>{selected.path}</strong><span>{selected.status}</span></header>}
            <pre>{selected?.patch || "No textual diff is available for this file."}</pre>
          </section>
        </div>
      ) : (
        <div className="changes-clean"><strong>Working tree is clean</strong><p>Create or run a task to begin making changes.</p></div>
      )}
      {props.embedded && (
        <footer className="changes-publish-bar">
          <span>{files.length ? "Review complete? Publish a draft pull request." : "No changes to publish."}</span>
          <button className="primary compact" type="button" disabled={!files.length || Boolean(props.error)} onClick={props.onPublish}>Review &amp; publish…</button>
        </footer>
      )}
    </section>
  );
}

export function countChangedFiles(changes: Changes): number {
  return parseChanges(changes).length;
}

function parseChanges(changes: Changes): ChangedFile[] {
  const status = new Map<string, string>();
  changes.status.split("\n").filter(Boolean).forEach((line) => {
    const path = line.slice(3).trim().split(" -> ").at(-1) || "";
    if (path) status.set(path, line.slice(0, 2).trim() || "M");
  });
  const patches = changes.diff.split(/(?=^diff --git )/m).filter((part) => part.startsWith("diff --git "));
  const files = patches.map((patch) => {
    const match = patch.match(/^diff --git a\/(.+) b\/(.+)$/m);
    const path = match?.[2] || match?.[1] || "Changed file";
    return fileRecord(path, status.get(path) || patchStatus(patch), patch);
  });
  status.forEach((value, path) => {
    if (!files.some((file) => file.path === path)) files.push(fileRecord(path, value, ""));
  });
  return files.sort((left, right) => left.path.localeCompare(right.path));
}

function fileRecord(path: string, status: string, patch: string): ChangedFile {
  const lines = patch.split("\n");
  return {
    path,
    status,
    patch,
    additions: lines.filter((line) => line.startsWith("+") && !line.startsWith("+++")).length,
    deletions: lines.filter((line) => line.startsWith("-") && !line.startsWith("---")).length,
  };
}

function patchStatus(patch: string): string {
  if (patch.includes("new file mode")) return "A";
  if (patch.includes("deleted file mode")) return "D";
  if (patch.includes("rename from")) return "R";
  return "M";
}
