import type { ComputeEnvironment } from "../api/environment-contracts";

export function EnvironmentCapacitySummary({ environments }: { environments: ComputeEnvironment[] }) {
  const available = environments.filter((value) => value.state === "available").length;
  const occupied = environments.filter((value) => value.state === "occupied" || value.state === "releasing").length;
  const attention = environments.filter((value) => value.state === "failed").length;
  const updating = environments.length - available - occupied - attention;

  return <section className="environment-capacity" aria-label="Environment capacity summary">
    <div className="environment-capacity-heading">
      <p><strong>{available} available</strong><span>of {environments.length} total</span></p>
      <div className="environment-capacity-legend">
        <CapacityLabel tone="available" label="Available" value={available} />
        <CapacityLabel tone="occupied" label="In use" value={occupied} />
        <CapacityLabel tone="attention" label="Attention" value={attention} />
        {updating > 0 && <CapacityLabel tone="updating" label="Updating" value={updating} />}
      </div>
    </div>
    {environments.length > 0 && <div
      className="environment-capacity-meter"
      role="img"
      aria-label={`${available} available, ${occupied} in use, ${attention} need attention${updating ? `, ${updating} updating` : ""}`}
    >
      {available > 0 && <span className="available" style={{ flexGrow: available }} />}
      {occupied > 0 && <span className="occupied" style={{ flexGrow: occupied }} />}
      {attention > 0 && <span className="attention" style={{ flexGrow: attention }} />}
      {updating > 0 && <span className="updating" style={{ flexGrow: updating }} />}
    </div>}
    {environments.length > 0 && available === 0 && <p className="environment-capacity-warning">No capacity is available. Stop a workspace or add another environment.</p>}
  </section>;
}

function CapacityLabel(props: { tone: string; label: string; value: number }) {
  return <span className={props.tone}><i aria-hidden="true" />{props.label} <strong>{props.value}</strong></span>;
}
