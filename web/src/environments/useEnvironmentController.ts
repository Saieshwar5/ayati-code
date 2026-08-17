import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { ComputeEnvironment, CreateComputeEnvironmentInput } from "../api/environment-contracts";

export function useEnvironmentController() {
  const [environments, setEnvironments] = useState<ComputeEnvironment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    const values = await api.environments();
    setEnvironments(values);
    return values;
  }, []);

  useEffect(() => {
    let current = true;
    api.environments().then(
      (values) => {
        if (!current) return;
        setEnvironments(values);
        setLoading(false);
      },
      (reason: Error) => {
        if (!current) return;
        setError(reason.message);
        setLoading(false);
      },
    );
    return () => { current = false; };
  }, []);

  const create = useCallback(async (input: CreateComputeEnvironmentInput) => {
    setError("");
    try {
      const value = await api.createEnvironmentCapacity(input);
      await refresh();
      return value;
    } catch (reason) {
      await refresh().catch(() => undefined);
      throw reason;
    }
  }, [refresh]);

  const repair = useCallback(async (id: string) => {
    setError("");
    try {
      const value = await api.repairEnvironmentCapacity(id);
      await refresh();
      return value;
    } catch (reason) {
      await refresh().catch(() => undefined);
      throw reason;
    }
  }, [refresh]);

  const remove = useCallback(async (id: string) => {
    setError("");
    await api.deleteEnvironmentCapacity(id);
    await refresh();
  }, [refresh]);

  return { environments, loading, error, setError, create, repair, remove };
}

export type EnvironmentController = ReturnType<typeof useEnvironmentController>;
