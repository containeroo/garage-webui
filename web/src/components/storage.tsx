import type { ReactNode } from "react";
import {
  Box,
  Database,
  FolderOpen,
  LayoutDashboard,
  LockKeyhole,
} from "lucide-react";
import { short, type Bucket } from "../api";

export function Metric({
  icon,
  label,
  value,
  detail,
  highlight = false,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  detail?: string;
  highlight?: boolean;
}) {
  return (
    <div className="metric">
      <div className="metric-top">
        <span>{label}</span>
        <div className="metric-icon">{icon}</div>
      </div>
      <div className={`metric-value ${highlight ? "green" : ""}`}>
        {highlight && <span className="dot" />}
        {value}
      </div>
      {detail && <small>{detail}</small>}
    </div>
  );
}
export function BucketRows({
  buckets,
  onSelect,
  loaded,
  loading,
}: {
  buckets: Bucket[];
  onSelect: (b: Bucket, tab?: "overview" | "permissions" | "browse") => void;
  loaded: boolean;
  loading: boolean;
}) {
  if (!loaded)
    return (
      <Empty
        text={loading ? "Loading buckets…" : "Buckets unavailable"}
        detail={loading ? "Please wait." : "Refresh to try again."}
      />
    );
  if (!buckets.length)
    return (
      <Empty text="No buckets yet" detail="Create a bucket to get started." />
    );
  return (
    <div className="table">
      <div className="table-head bucket-columns">
        <span>BUCKET NAME</span>
        <span>BUCKET ID</span>
        <span>CREATED</span>
        <span className="bucket-actions-heading">ACTIONS</span>
      </div>
      {buckets.map((bucket) => (
        <div className="table-row bucket-columns" key={bucket.id}>
          <button
            className="item-name bucket-name-button"
            onClick={() => onSelect(bucket, "overview")}
          >
            <div className="item-icon">
              <Database size={16} />
            </div>
            <strong>{bucket.globalAliases?.[0] || "Unnamed bucket"}</strong>
          </button>
          <code>{short(bucket.id)}</code>
          <span>
            {bucket.created
              ? new Date(bucket.created).toLocaleDateString()
              : "—"}
          </span>
          <div className="bucket-actions">
            {(
              [
                ["Overview", "overview", LayoutDashboard],
                ["Permissions", "permissions", LockKeyhole],
                ["Manage objects", "browse", FolderOpen],
              ] as const
            ).map(([label, tab, Icon]) => (
              <button
                key={tab}
                className="icon-button"
                title={label}
                aria-label={`${label}: ${bucket.globalAliases?.[0] || bucket.id}`}
                onClick={() => onSelect(bucket, tab)}
              >
                <Icon size={16} />
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
export function Empty({ text, detail }: { text: string; detail?: string }) {
  return (
    <div className="empty">
      <Box size={22} />
      <strong>{text}</strong>
      {detail && <span>{detail}</span>}
    </div>
  );
}
