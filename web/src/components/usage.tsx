import { short, type Bucket } from "../api";
import { useBucketUsage } from "../hooks/use-bucket-usage";
import { formatBytes } from "../lib/format";

/** Usage loads bucket usage summaries and reports aggregate storage and object counts. */
export function Usage({
  buckets,
  loaded,
  busy,
  onSelect,
  onTotals,
  onViewAll,
}: {
  /** Buckets accessible to the current credential. */
  buckets: Bucket[];
  /** Whether the initial resource list loaded successfully. */
  loaded: boolean;
  /** Whether a request or mutation is currently in progress. */
  busy: boolean;
  /** Opens the chosen bucket and optional detail tab. */
  onSelect: (bucket: Bucket) => void;
  /** Navigates from the summary to the full bucket list. */
  onViewAll: () => void;
  /** Reports aggregate usage, or undefined while usage is unavailable. */
  onTotals: (
    totals:
      | {
          /** Total stored object bytes. */
          bytes: number;
          /** Total number of stored objects. */
          objects: number;
        }
      | undefined,
  ) => void;
}) {
  const { data, failed } = useBucketUsage(buckets, loaded, busy, onTotals);
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
