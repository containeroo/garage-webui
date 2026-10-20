import { AdminOperation } from "../admin-operation";
import { CredentialKind } from "../domain";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  allows,
  errorMessage,
  get,
  type Bucket,
  type Health,
  type AccessKey,
  type Session,
} from "../api";

// useWorkspaceData loads independent workspace summaries and discards superseded requests.
export function useWorkspaceData(session: Session) {
  const [health, setHealth] = useState<Health>();
  const [buckets, setBuckets] = useState<Bucket[]>([]);
  const [keys, setKeys] = useState<AccessKey[]>([]);
  const [bucketsLoaded, setBucketsLoaded] = useState(false);
  const [keysLoaded, setKeysLoaded] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const pending = useRef<AbortController | null>(null);

  // load keeps partial successes while preventing older refreshes from replacing newer data.
  const load = useCallback(async () => {
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    const options = { signal: controller.signal };
    setBusy(true);
    setError("");

    const results = await Promise.allSettled([
      allows(session, AdminOperation.GetClusterHealth)
        ? get<Health>(AdminOperation.GetClusterHealth, "", options)
        : Promise.resolve(undefined),
      session.mode === CredentialKind.S3 ||
      allows(session, AdminOperation.ListBuckets)
        ? get<Bucket[]>(AdminOperation.ListBuckets, "", options)
        : Promise.resolve([]),
      allows(session, AdminOperation.ListKeys)
        ? get<AccessKey[]>(AdminOperation.ListKeys, "", options)
        : Promise.resolve([]),
    ]);
    if (controller.signal.aborted) return;

    if (results[0].status === "fulfilled") setHealth(results[0].value);
    if (results[1].status === "fulfilled") setBuckets(results[1].value);
    if (results[2].status === "fulfilled") setKeys(results[2].value);
    setBucketsLoaded(results[1].status === "fulfilled");
    setKeysLoaded(results[2].status === "fulfilled");
    setLoaded(true);
    const failure = results.find((result) => result.status === "rejected");
    if (failure?.status === "rejected")
      setError(
        `could not load all Garage data: ${errorMessage(failure.reason)}`,
      );
    setBusy(false);
  }, [session]);

  useEffect(() => {
    void load();
    return () => pending.current?.abort();
  }, [load]);

  return {
    health,
    buckets,
    keys,
    bucketsLoaded,
    keysLoaded,
    loaded,
    busy,
    error,
    setError,
    load,
  };
}
