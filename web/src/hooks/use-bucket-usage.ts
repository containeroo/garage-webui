import { useEffect, useState } from "react";
import { get, type Bucket, type BucketDetails } from "../api";

/** Complete storage and object counters for the bucket usage summary. */
export type UsageTotals = {
  /** Total stored bytes across all successfully loaded buckets. */
  bytes: number;
  /** Total stored objects across all successfully loaded buckets. */
  objects: number;
};

/** Bucket metadata whose required usage counters have been validated. */
type BucketUsage = BucketDetails & UsageTotals;

// hasUsageCounters rejects incomplete, negative, or non-finite usage data from Garage.
function hasUsageCounters(bucket: BucketDetails): bucket is BucketUsage {
  return (
    !!bucket &&
    typeof bucket.id === "string" &&
    typeof bucket.bytes === "number" &&
    Number.isFinite(bucket.bytes) &&
    bucket.bytes >= 0 &&
    typeof bucket.objects === "number" &&
    Number.isFinite(bucket.objects) &&
    bucket.objects >= 0
  );
}

// useBucketUsage loads usage in bounded batches and discards work after navigation or refresh.
export function useBucketUsage(
  buckets: Bucket[],
  loaded: boolean,
  busy: boolean,
  onTotals: (totals: UsageTotals | undefined) => void,
) {
  const [data, setData] = useState<BucketUsage[]>();
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!loaded || busy) return;
    const controller = new AbortController();
    setData(undefined);
    onTotals(undefined);
    setFailed(false);

    // load limits concurrent API requests while requiring complete data for aggregate totals.
    async function load() {
      const usage: BucketUsage[] = [];
      const concurrency = 6;
      for (let offset = 0; offset < buckets.length; offset += concurrency) {
        if (controller.signal.aborted) return;
        const batch = await Promise.all(
          buckets
            .slice(offset, offset + concurrency)
            .map((bucket) =>
              get<BucketDetails>(
                "GetBucketInfo",
                `?id=${encodeURIComponent(bucket.id)}`,
                { signal: controller.signal },
              ),
            ),
        );
        for (const bucket of batch) {
          if (!hasUsageCounters(bucket)) throw new Error("invalid usage data");
          usage.push(bucket);
        }
      }
      if (controller.signal.aborted) return;
      setData(usage);
      onTotals(
        usage.reduce(
          (totals, bucket) => ({
            bytes: totals.bytes + bucket.bytes,
            objects: totals.objects + bucket.objects,
          }),
          { bytes: 0, objects: 0 },
        ),
      );
    }

    void load().catch(() => {
      if (!controller.signal.aborted) setFailed(true);
    });
    return () => controller.abort();
  }, [buckets, loaded, busy, onTotals]);

  return { data, failed };
}
