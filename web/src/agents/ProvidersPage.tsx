import { useEffect, useState } from "react";
import type { ProviderConnectionInput, ProviderDefinition } from "../api/contracts";
import type { AgentController } from "./useAgentController";

const emptyConnection: ProviderConnectionInput = { api_key: "", default_model: "" };

export function ProvidersPage({ controller }: { controller: AgentController }) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const selected = controller.providers.find((provider) => provider.id === selectedID);
  const configured = controller.providers.filter((provider) => provider.configured).length;
  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame">
        <header className="agent-page-heading">
          <div>
            <p className="eyebrow">Model access</p>
            <h1>Providers</h1>
            <p className="muted">Configure global model connections. API keys stay in Ayati’s private controller configuration.</p>
          </div>
          <span className="provider-summary">{configured} configured</span>
        </header>
        {controller.error && <div className="error" role="alert">{controller.error}</div>}
        <div className={`provider-layout${selected ? " editing" : ""}`}>
          {controller.loading ? <p className="muted">Loading providers…</p> : (
            <div className="provider-card-grid">
              {controller.providers.map((provider) => (
                <ProviderCard key={provider.id} provider={provider} onConfigure={() => setSelectedID(provider.id)} />
              ))}
            </div>
          )}
          {selected && <ConnectionEditor
            key={`${selected.id}:${selected.default_model || ""}`}
            provider={selected}
            controller={controller}
            onClose={() => setSelectedID(null)}
          />}
        </div>
        <div className="provider-foundation-note">
          <strong>Credentials remain controller-owned</strong>
          <p>Keys are never returned by this API and are not placed in SQLite, sessions, prompts, logs or workspace containers.</p>
        </div>
      </div>
    </section>
  );
}

function ProviderCard(props: { provider: ProviderDefinition; onConfigure: () => void }) {
  const provider = props.provider;
  return <article className="provider-card">
    <div className="provider-card-mark" aria-hidden="true">◎</div>
    <div>
      <span className={`provider-state${provider.configured ? " configured" : ""}`}>
        {provider.configured ? "Configured" : "Not configured"}
      </span>
      <h2>{provider.name}</h2>
      <p>{provider.protocol}</p>
      {provider.default_model && <p className="provider-model">Default · {provider.default_model}</p>}
    </div>
    <button className="quiet compact" type="button" disabled={!provider.configurable} onClick={props.onConfigure}>
      {provider.configured ? "Configure" : "Set up"}
    </button>
  </article>;
}

function ConnectionEditor(props: {
  provider: ProviderDefinition;
  controller: AgentController;
  onClose: () => void;
}) {
  const [input, setInput] = useState<ProviderConnectionInput>({
    ...emptyConnection, default_model: props.provider.default_model || "",
  });
  const [busy, setBusy] = useState<"save" | "test" | "remove" | "">("");
  const [error, setError] = useState("");
  const [verified, setVerified] = useState(false);

  useEffect(() => {
    setVerified(false);
  }, [input.api_key, input.default_model]);

  async function save() {
    setBusy("save"); setError("");
    try {
      await props.controller.configureProvider(props.provider.id, input);
      setInput((current) => ({ ...current, api_key: "" }));
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy("");
    }
  }

  async function test() {
    setBusy("test"); setError(""); setVerified(false);
    try {
      await props.controller.testProvider(props.provider.id, input);
      setVerified(true);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy("");
    }
  }

  async function remove() {
    setBusy("remove"); setError("");
    try {
      if (await props.controller.removeProvider(props.provider)) props.onClose();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy("");
    }
  }

  const needsKey = !props.provider.configured && !input.api_key.trim();
  return <aside className="provider-editor" aria-label={`Configure ${props.provider.name}`}>
    <header><div><p className="eyebrow">Provider connection</p><h2>{props.provider.name}</h2></div><button className="quiet compact" type="button" onClick={props.onClose}>Close</button></header>
    <label>API key<input type="password" autoComplete="off" value={input.api_key} placeholder={props.provider.configured ? "Leave blank to keep the saved key" : "Required"} onChange={(event) => setInput({ ...input, api_key: event.target.value })} /></label>
    <p className="agent-field-note">The saved key is never sent back to this page.</p>
    <label>Default model<input value={input.default_model} placeholder="Model ID" onChange={(event) => setInput({ ...input, default_model: event.target.value })} /></label>
    {error && <div className="error" role="alert">{error}</div>}
    {verified && <div className="provider-verified" role="status">Connection verified</div>}
    <div className="provider-editor-actions">
      {props.provider.supports_test && <button className="quiet" type="button" disabled={busy !== "" || needsKey || !input.default_model.trim()} onClick={() => void test()}>{busy === "test" ? "Testing…" : "Test connection"}</button>}
      <button className="primary" type="button" disabled={busy !== "" || needsKey || !input.default_model.trim()} onClick={() => void save()}>{busy === "save" ? "Saving…" : "Save connection"}</button>
    </div>
    {props.provider.configured && <button className="quiet compact danger-text provider-remove" type="button" disabled={busy !== ""} onClick={() => void remove()}>{busy === "remove" ? "Removing…" : "Remove saved connection"}</button>}
  </aside>;
}
