import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";

export function useConfig() {
  const ws = useWs();
  const [config, setConfig] = useState<Record<string, unknown> | null>(null);
  const [hash, setHash] = useState("");
  const [configPath, setConfigPath] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!ws.isConnected) return;
    setLoading(true);
    try {
      const res = await ws.call<{
        config: Record<string, unknown>;
        hash: string;
        path: string;
      }>(Methods.CONFIG_GET);
      setConfig(res.config);
      setHash(res.hash);
      setConfigPath(res.path);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [ws]);

  useEffect(() => {
    load();
  }, [load]);

  const applyRaw = useCallback(
    async (raw: string) => {
      setSaving(true);
      setError(null);
      try {
        const res = await ws.call<{ hash: string }>(Methods.CONFIG_APPLY, {
          raw,
          baseHash: hash,
        });
        setHash(res.hash);
        load();
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to apply config");
        throw err;
      } finally {
        setSaving(false);
      }
    },
    [ws, hash, load],
  );

  const patch = useCallback(
    async (updates: Record<string, unknown>) => {
      setSaving(true);
      setError(null);
      try {
        const res = await ws.call<{ hash: string }>(Methods.CONFIG_PATCH, {
          raw: JSON.stringify(updates),
          baseHash: hash,
        });
        if (res?.hash) setHash(res.hash);
        load();
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to patch config");
        throw err;
      } finally {
        setSaving(false);
      }
    },
    [ws, hash, load],
  );

  return { config, hash, configPath, loading, saving, error, refresh: load, applyRaw, patch };
}
