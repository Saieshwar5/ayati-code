import type { AgentController } from "./useAgentController";

export function ProvidersPage({ controller }: { controller: AgentController }) {
  const configured = controller.providers.filter((provider) => provider.configured).length;
  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame">
        <header className="agent-page-heading">
          <div>
            <p className="eyebrow">Model access</p>
            <h1>Providers</h1>
            <p className="muted">Provider definitions are global. Credentials stay controller-owned and never enter workspace sandboxes.</p>
          </div>
          <span className="provider-summary">{configured} configured</span>
        </header>
        {controller.error && <div className="error" role="alert">{controller.error}</div>}
        {controller.loading ? <p className="muted">Loading providers…</p> : (
          <div className="provider-card-grid">
            {controller.providers.map((provider) => (
              <article className="provider-card" key={provider.id}>
                <div className="provider-card-mark" aria-hidden="true">◎</div>
                <div>
                  <span className={`provider-state${provider.configured ? " configured" : ""}`}>
                    {provider.configured ? "Configured" : "Not configured"}
                  </span>
                  <h2>{provider.name}</h2>
                  <p>{provider.protocol}</p>
                </div>
                <small>Built-in provider definition</small>
              </article>
            ))}
          </div>
        )}
        <div className="provider-foundation-note">
          <strong>Provider foundation ready</strong>
          <p>Connection management and additional protocol adapters will be added in focused provider branches.</p>
        </div>
      </div>
    </section>
  );
}
