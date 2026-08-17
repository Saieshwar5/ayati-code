import { useCallback, useEffect, useState } from "react";
import type { AgentDefinition, AgentInput, ProviderConnectionInput, ProviderDefinition } from "../api/contracts";
import { api } from "../api/client";

export function useAgentController(enabled: boolean) {
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [archivedAgents, setArchivedAgents] = useState<AgentDefinition[]>([]);
  const [providers, setProviders] = useState<ProviderDefinition[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setError("");
    const [active, archived, availableProviders] = await Promise.all([
      api.agents(), api.archivedAgents(), api.providers(),
    ]);
    setAgents(active);
    setArchivedAgents(archived);
    setProviders(availableProviders);
    return active;
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let current = true;
    setLoading(true);
    Promise.all([api.agents(), api.archivedAgents(), api.providers()])
      .then(([active, archived, availableProviders]) => {
        if (!current) return;
        setAgents(active);
        setArchivedAgents(archived);
        setProviders(availableProviders);
        setError("");
      })
      .catch((reason: Error) => {
        if (current) setError(reason.message);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [enabled]);

  const create = useCallback(async (input: AgentInput) => {
    const created = await api.createAgent(input);
    await refresh();
    return created;
  }, [refresh]);

  const update = useCallback(async (id: string, input: AgentInput) => {
    const updated = await api.updateAgent(id, input);
    await refresh();
    return updated;
  }, [refresh]);

  const makeDefault = useCallback(async (id: string) => {
    await api.setDefaultAgent(id);
    await refresh();
  }, [refresh]);

  const duplicate = useCallback(async (id: string) => {
    const created = await api.duplicateAgent(id);
    await refresh();
    return created;
  }, [refresh]);

  const archive = useCallback(async (definition: AgentDefinition) => {
    if (!window.confirm(`Archive “${definition.name}”?\n\nSessions using it will return to the default agent.`)) {
      return false;
    }
    await api.archiveAgent(definition.id);
    await refresh();
    return true;
  }, [refresh]);

  const restore = useCallback(async (id: string) => {
    await api.restoreAgent(id);
    await refresh();
  }, [refresh]);

  const configureProvider = useCallback(async (id: string, input: ProviderConnectionInput) => {
    await api.configureProvider(id, input);
    await refresh();
  }, [refresh]);

  const testProvider = useCallback(async (id: string, input: ProviderConnectionInput) => {
    await api.testProvider(id, input);
  }, []);

  const removeProvider = useCallback(async (provider: ProviderDefinition) => {
    if (!window.confirm(`Remove the saved ${provider.name} connection?\n\nAgents using it will stop working until it is configured again.`)) {
      return false;
    }
    await api.removeProvider(provider.id);
    await refresh();
    return true;
  }, [refresh]);

  return {
    agents, archivedAgents, providers, loading, error, refresh,
    create, update, makeDefault, duplicate, archive, restore,
    configureProvider, testProvider, removeProvider,
  };
}

export type AgentController = ReturnType<typeof useAgentController>;
