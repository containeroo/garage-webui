import { useEffect, useState } from "react";
import { get, short, type Bucket, type Details } from "../api";

export function formatBytes(value: number) {
  if (value < 1000) return `${value} B`;
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1000)), 4);
  return `${(value / 1000 ** unit).toFixed(1)} ${["B", "kB", "MB", "GB", "TB"][unit]}`;
}

export function Usage({
  buckets,
  loaded,
  busy,
  onSelect,
  onTotals,
  onViewAll,
}: {
  buckets: Bucket[];
  loaded: boolean;
  busy: boolean;
  onSelect: (bucket: Bucket) => void;
  onViewAll: () => void;
  onTotals: (totals: { bytes: number; objects: number } | undefined) => void;
}) {
  const [data, setData] = useState<Details[]>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (!loaded || busy) return;
    let cancelled = false;
    setData(undefined);
    onTotals(undefined);
    setFailed(false);
    Promise.all(
      buckets.map((b) =>
        get<Details>("GetBucketInfo", `?id=${encodeURIComponent(b.id)}`),
      ),
    )
      .then((value) => {
        if (
          value.some(
            (item) =>
              !item ||
              typeof item.id !== "string" ||
              typeof item.bytes !== "number" ||
              typeof item.objects !== "number",
          )
        )
          throw new Error("invalid usage data");
        if (!cancelled) {
          setData(value);
          onTotals({
            bytes: value.reduce((sum, b) => sum + (b.bytes || 0), 0),
            objects: value.reduce((sum, b) => sum + (b.objects || 0), 0),
          });
        }
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [buckets, loaded, busy, onTotals]);
  const sorted = [...(data || [])].sort(
    (a, b) => (b.bytes || 0) - (a.bytes || 0),
  );
  const max = sorted[0]?.bytes || 0;
  return (
    <section className="panel usage-panel">
      <div className="panel-head">
        <h2>Storage by bucket</h2>
        <button className="text-button" onClick={onViewAll}>
          View all
        </button>
      </div>
      {failed ? (
        <p className="usage-message">
          Usage unavailable. Refresh to try again.
        </p>
      ) : !data ? (
        <p className="usage-message">Loading usage…</p>
      ) : !data.length ? (
        <p className="usage-message">No buckets yet.</p>
      ) : (
        <div className="usage-list">
          {sorted.map((b) => (
            <button
              key={b.id}
              className="usage-row"
              onClick={() => {
                const bucket = buckets.find((item) => item.id === b.id);
                if (bucket) onSelect(bucket);
              }}
            >
              <span className="usage-name" title={b.globalAliases?.[0] || b.id}>
                {b.globalAliases?.[0] || short(b.id)}
              </span>
              <span className="usage-track">
                <span
                  style={{
                    width: `${max ? Math.max(1, ((b.bytes || 0) / max) * 100) : 0}%`,
                  }}
                />
              </span>
              <span className="usage-values">
                <strong>{formatBytes(b.bytes || 0)}</strong>
                <span>{(b.objects || 0).toLocaleString()} objects</span>
              </span>
            </button>
          ))}
        </div>
      )}
    </section>
  );
}
