import { useMemo, useState } from "react";
import type { Repository } from "../api/contracts";

const suggestionLimit = 5;

interface RepositoryPickerProps {
  repositories: Repository[];
  recentRepositories?: string[];
  value: string;
  onChange: (repository: string) => void;
}

export function RepositoryPicker(props: RepositoryPickerProps) {
  const [query, setQuery] = useState("");
  const [browseQuery, setBrowseQuery] = useState("");
  const [browsing, setBrowsing] = useState(false);
  const [editing, setEditing] = useState(false);
  const hasRecentRepositories = Boolean(props.recentRepositories?.length);
  const selected = props.repositories.find((repository) => repository.full_name === props.value);
  const ordered = useMemo(
    () => orderRepositories(props.repositories, props.recentRepositories || []),
    [props.repositories, props.recentRepositories],
  );
  const suggestions = filterRepositories(ordered, query).slice(0, suggestionLimit);
  const browseResults = filterRepositories(ordered, browseQuery);

  function choose(repository: string) {
    props.onChange(repository);
    setQuery("");
    setBrowseQuery("");
    setBrowsing(false);
    setEditing(false);
  }

  if (selected && !editing) {
    return (
      <aside className="repository-picker selected-repository" aria-label="Selected repository">
        <div>
          <span className="repository-selected-marker" aria-hidden="true" />
          <span><strong>{selected.full_name}</strong><small>{selected.default_branch} · {selected.private ? "Private" : "Public"}</small></span>
        </div>
        <button className="quiet compact" type="button" onClick={() => setEditing(true)}>Change</button>
      </aside>
    );
  }

  return (
    <aside className="repository-picker" aria-labelledby="repository-picker-title">
      <div className="repository-picker-heading">
        <div>
          <h2 id="repository-picker-title">{hasRecentRepositories ? "Recent repositories" : "Repositories"}</h2>
          <span>{props.repositories.length}</span>
        </div>
        <label className="repository-search">
          <span className="sr-only">Search repositories</span>
          <input type="search" value={query} placeholder="Search repositories" onChange={(event) => setQuery(event.target.value)} />
        </label>
      </div>
      {!browsing && (
        <>
          <RepositoryOptions repositories={suggestions} value={props.value} emptyMessage={emptyMessage(props.repositories, suggestions, query)} onChange={choose} />
          <div className="repository-picker-footer">
            <span>{query ? `${suggestions.length} shown` : `Showing ${Math.min(suggestionLimit, ordered.length)} ${hasRecentRepositories ? "recent" : "suggestions"}`}</span>
            <button className="quiet compact" type="button" onClick={() => setBrowsing(true)}>Browse all</button>
          </div>
        </>
      )}
      {browsing && (
        <div className="repository-browser-backdrop">
          <section className="repository-browser" role="dialog" aria-modal="true" aria-labelledby="repository-browser-title" onKeyDown={(event) => { if (event.key === "Escape") setBrowsing(false); }}>
            <header>
              <div><h2 id="repository-browser-title">All repositories</h2><p>{props.repositories.length} available</p></div>
              <button className="quiet compact" type="button" aria-label="Close repository browser" onClick={() => setBrowsing(false)}>Close</button>
            </header>
            <label className="repository-search">
              <span className="sr-only">Search all repositories</span>
              <input autoFocus type="search" value={browseQuery} placeholder="Search all repositories" onChange={(event) => setBrowseQuery(event.target.value)} />
            </label>
            <RepositoryOptions repositories={browseResults} value={props.value} emptyMessage={browseResults.length ? "" : "No repositories match your search."} onChange={choose} />
          </section>
        </div>
      )}
    </aside>
  );
}

function RepositoryOptions(props: { repositories: Repository[]; value: string; emptyMessage: string; onChange: (repository: string) => void }) {
  return (
    <fieldset className="repository-options">
      <legend className="sr-only">Repository</legend>
      {props.emptyMessage && <p>{props.emptyMessage}</p>}
      {props.repositories.map((repository) => (
        <label className={props.value === repository.full_name ? "selected" : ""} key={repository.id}>
          <input type="radio" name="repository" value={repository.full_name} aria-label={repository.full_name} checked={props.value === repository.full_name} onChange={() => props.onChange(repository.full_name)} />
          <span><strong>{repository.full_name}</strong><small>{repository.default_branch} · {repository.private ? "Private" : "Public"}</small></span>
          <i aria-hidden="true" />
        </label>
      ))}
    </fieldset>
  );
}

function orderRepositories(repositories: Repository[], recentNames: string[]): Repository[] {
  const order = new Map(recentNames.map((name, index) => [name, index]));
  return [...repositories].sort((left, right) => {
    const leftIndex = order.get(left.full_name) ?? Number.MAX_SAFE_INTEGER;
    const rightIndex = order.get(right.full_name) ?? Number.MAX_SAFE_INTEGER;
    return leftIndex - rightIndex;
  });
}

function filterRepositories(repositories: Repository[], query: string): Repository[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return repositories;
  return repositories.filter((repository) => repository.full_name.toLowerCase().includes(normalized));
}

function emptyMessage(all: Repository[], visible: Repository[], query: string): string {
  if (!all.length) return "No installed repositories.";
  if (!visible.length) return `No repositories match “${query}”.`;
  return "";
}
