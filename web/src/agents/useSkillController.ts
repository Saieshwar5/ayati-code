import { useCallback, useEffect, useState } from "react";
import type { SkillDefinition, SkillInput } from "../api/contracts";
import { api } from "../api/client";

export function useSkillController(enabled: boolean) {
  const [skills, setSkills] = useState<SkillDefinition[]>([]);
  const [archivedSkills, setArchivedSkills] = useState<SkillDefinition[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setError("");
    try {
      const [active, archived] = await Promise.all([api.skills(), api.archivedSkills()]);
      setSkills(active);
      setArchivedSkills(archived);
    } catch (reason) {
      setError((reason as Error).message);
      throw reason;
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let current = true;
    setLoading(true);
    Promise.all([api.skills(), api.archivedSkills()])
      .then(([active, archived]) => {
        if (!current) return;
        setSkills(active);
        setArchivedSkills(archived);
        setError("");
      })
      .catch((reason: Error) => {
        if (current) setError(reason.message);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => { current = false; };
  }, [enabled]);

  const create = useCallback(async (input: SkillInput) => {
    const value = await api.createSkill(input);
    await refresh();
    return value;
  }, [refresh]);

  const update = useCallback(async (id: string, input: SkillInput) => {
    const value = await api.updateSkill(id, input);
    await refresh();
    return value;
  }, [refresh]);

  const archive = useCallback(async (skill: SkillDefinition) => {
    if (!window.confirm(`Archive “${skill.name}”?`)) return false;
    await api.archiveSkill(skill.id);
    await refresh();
    return true;
  }, [refresh]);

  const restore = useCallback(async (id: string) => {
    await api.restoreSkill(id);
    await refresh();
  }, [refresh]);

  return { skills, archivedSkills, loading, error, create, update, archive, restore, refresh };
}

export type SkillController = ReturnType<typeof useSkillController>;
