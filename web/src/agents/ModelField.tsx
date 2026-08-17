import { useEffect, useId, useState } from "react";
import type { ProviderDefinition, ProviderModel } from "../api/contracts";
import { api } from "../api/client";

interface ModelFieldProps {
  provider?: ProviderDefinition;
  label?: string;
  value: string;
  disabled?: boolean;
  placeholder: string;
  onChange: (value: string) => void;
}

export function ModelField(props: ModelFieldProps) {
  const fieldID = useId();
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const canDiscover = Boolean(props.provider?.configured && props.provider.supports_models);

  useEffect(() => {
    setModels([]);
    setLoaded(false);
    setError("");
  }, [props.provider?.id]);

  async function load() {
    if (!props.provider || !canDiscover || loading) return;
    setLoading(true);
    setError("");
    try {
      setModels(await api.providerModels(props.provider.id));
      setLoaded(true);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setLoading(false);
    }
  }

  let note = "Enter a model ID manually.";
  if (props.provider?.supports_models && !props.provider.configured) {
    note = "Save the provider connection before browsing models.";
  } else if (canDiscover && loaded) {
    note = models.length === 0 ? "No models were returned; manual entry remains available." : `${models.length} models available · manual entry remains available.`;
  } else if (canDiscover) {
    note = "Browse models or enter an ID manually.";
  }

  return <div className="model-field">
    <label htmlFor={fieldID}>{props.label || "Model"}</label>
    <div className="model-field-input">
      <input
        id={fieldID}
        list={models.length > 0 ? `${fieldID}-models` : undefined}
        value={props.value}
        disabled={props.disabled}
        placeholder={props.placeholder}
        onFocus={() => { if (!loaded) void load(); }}
        onChange={(event) => props.onChange(event.target.value)}
      />
      {canDiscover && !props.disabled && <button className="quiet compact" type="button" disabled={loading} onClick={() => void load()}>{loading ? "Loading…" : loaded ? "Refresh" : "Browse"}</button>}
    </div>
    {models.length > 0 && <datalist id={`${fieldID}-models`}>{models.map((model) => <option key={model.id} value={model.id} />)}</datalist>}
    <small className={error ? "error-text" : ""}>{error || note}</small>
  </div>;
}
