import type { ReactNode } from "react";
import {
  Box,
  Database,
  FolderOpen,
  LayoutDashboard,
  LockKeyhole,
} from "lucide-react";
import { short, type Bucket } from "../api";

/** Metric renders a labeled workspace summary with an optional health indicator. */
export function Metric({
  icon,
  label,
  value,
  detail,
  highlight = false,
}: {
  /** Decorative icon associated with the displayed metric. */
  icon: ReactNode;
  /** Visible descriptive label for the value. */
  label: string;
  /** Formatted value displayed by the metric. */
  value: string;
  /** Optional context or recovery guidance beneath the main text. */
  detail?: string;
  /** Whether the metric should receive a positive health indicator. */
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
/** BucketRows renders loading, empty, and navigable bucket-list states. */
export function BucketRows({
  buckets,
  onSelect,
  loaded,
  loading,
  browseOnly = false,
}: {
  /** Buckets accessible to the current credential. */
  buckets: Bucket[];
  /** Opens the chosen bucket and optional detail tab. */
  onSelect: (b: Bucket, tab?: "overview" | "permissions" | "browse") => void;
  /** Whether the initial resource list loaded successfully. */
  loaded: boolean;
  /** Whether the list is currently awaiting a response. */
  loading: boolean;
  /** Whether bucket actions are restricted to object browsing. */
  browseOnly?: boolean;
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
            onClick={() => onSelect(bucket, browseOnly ? "browse" : "overview")}
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
            )
              .filter(([, tab]) => !browseOnly || tab === "browse")
              .map(([label, tab, Icon]) => (
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
/** Empty presents a consistent no-data state with optional recovery guidance. */
export function Empty({
  text,
  detail,
}: {
  /** Primary message for an empty or unavailable-data state. */
  text: string;
  /** Optional context or recovery guidance beneath the main text. */
  detail?: string;
}) {
  return (
    <div className="empty">
      <Box size={22} />
      <strong>{text}</strong>
      {detail && <span>{detail}</span>}
    </div>
  );
}
