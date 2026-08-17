interface PlaceholderPageProps {
  eyebrow: string;
  title: string;
  description: string;
}

export function PlaceholderPage(props: PlaceholderPageProps) {
  return (
    <section className="page-scroll">
      <div className="page-frame narrow">
        <div className="placeholder-card">
          <span className="empty-glyph" aria-hidden="true">✦</span>
          <p className="eyebrow">{props.eyebrow}</p>
          <h1>{props.title}</h1>
          <p className="muted">{props.description}</p>
          <span className="coming-label">Planned</span>
        </div>
      </div>
    </section>
  );
}
