import { AdminOperation } from "../admin-operation";
import type { FormEvent } from "react";
import { useQuestions } from "./ui/questions";
import { Activity, HardDrive, Network, ShieldCheck } from "lucide-react";
import { Button } from "./ui/button";
import { DialogPanel } from "./ui/dialog";
import { Metric } from "./storage";
import { useCallback, useEffect, useState } from "react";
import {
  allows,
  errorMessage,
  get,
  post,
  short,
  type Health,
  type Session,
} from "../api";
import { formatBytes } from "../lib/format";

/** A node's assigned storage zone, capacity, and descriptive tags. */
type ClusterRole = {
  /** Failure-domain zone assigned to the node. */
  zone: string;
  /** Storage capacity in bytes; null marks a gateway node. */
  capacity: number | null;
  /** Descriptive tags attached to the node's layout role. */
  tags: string[];
};
/** A connected or known Garage node and its assigned layout role. */
type ClusterNode = {
  /** Stable Garage identifier for this resource. */
  id: string;
  /** Node hostname when advertised by Garage. */
  hostname?: string;
  /** Advertised RPC address of the node. */
  addr?: string;
  /** Garage software version advertised by the node. */
  garageVersion?: string;
  /** Whether this node is currently reachable. */
  isUp: boolean;
  /** Whether this node is draining stored data. */
  draining: boolean;
  /** Current node assignment; absent for an unassigned node. */
  role?: ClusterRole | null;
};
/** The active cluster layout version and known node status. */
type ClusterStatus = {
  /** Version of the layout currently used by the cluster. */
  layoutVersion: number;
  /** Known cluster nodes and their connectivity metadata. */
  nodes: ClusterNode[];
};
/** The active layout and staged assignments awaiting application. */
type ClusterLayout = {
  /** Layout version used to serialize assignment changes. */
  version: number;
  /** Node assignments or removals waiting to be applied. */
  stagedRoleChanges: {
    /** Stable Garage identifier for this resource. */
    id: string;
    /** Failure-domain zone assigned to the node. */
    zone?: string;
    /** Storage capacity in bytes; null marks a gateway node. */
    capacity?: number | null;
    /** Descriptive tags attached to the node's layout role. */
    tags?: string[];
    /** Whether this staged change removes the node assignment. */
    remove?: boolean;
  }[];
  /** Storage and gateway roles in the current layout. */
  roles?: ({
    /** Stable Garage identifier for this resource. */
    id: string;
  } & ClusterRole)[];
  /** Current layout-wide placement parameters. */
  parameters?: {
    /** Required number of failure-domain zones or maximum available redundancy. */
    zoneRedundancy:
      | "maximum"
      | {
          /** Minimum number of zones required for each partition. */
          atLeast: number;
        };
  };
  /** Placement parameters staged for the next layout version. */
  stagedParameters?: {
    /** Required number of failure-domain zones or maximum available redundancy. */
    zoneRedundancy:
      | "maximum"
      | {
          /** Minimum number of zones required for each partition. */
          atLeast: number;
        };
  } | null;
};
/** Messages or an error returned when previewing staged layout changes. */
type LayoutPreview = {
  /** Failure message suitable for displaying beside the affected action. */
  error?: string;
  /** Explanatory messages returned by Garage or shown in a prompt. */
  message?: string[];
};

/** Cluster presents node status and stages explicitly confirmed layout changes. */
export function Cluster({
  session,
  health,
}: {
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
  /** Latest cluster health summary when available to this credential. */
  health?: Health;
}) {
  const { confirm } = useQuestions();
  const [status, setStatus] = useState<ClusterStatus>();
  const [layout, setLayout] = useState<ClusterLayout>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [address, setAddress] = useState("");
  const [editing, setEditing] = useState<ClusterNode>();
  const [removing, setRemoving] = useState<ClusterNode>();
  const [removeError, setRemoveError] = useState("");
  const [zone, setZone] = useState("");
  const [capacity, setCapacity] = useState("");
  const [unit, setUnit] = useState("GB");
  const [assignmentError, setAssignmentError] = useState("");
  const [redundancy, setRedundancy] = useState("");
  const [gateway, setGateway] = useState(false);
  const [tags, setTags] = useState("");
  const [preview, setPreview] = useState<LayoutPreview>();
  const [busy, setBusy] = useState(false);
  /** refresh retrieves the current view's authoritative data before updating its state. */
  const refresh = useCallback(async () => {
    const [statusResult, layoutResult] = await Promise.allSettled([
      allows(session, AdminOperation.GetClusterStatus)
        ? get<ClusterStatus>(AdminOperation.GetClusterStatus)
        : Promise.resolve(undefined),
      allows(session, AdminOperation.GetClusterLayout)
        ? get<ClusterLayout>(AdminOperation.GetClusterLayout)
        : Promise.resolve(undefined),
    ]);
    if (statusResult.status === "fulfilled") setStatus(statusResult.value);
    if (layoutResult.status === "fulfilled") setLayout(layoutResult.value);
    const failure = [statusResult, layoutResult].find(
      (result) => result.status === "rejected",
    );
    if (failure?.status === "rejected") throw failure.reason;
  }, [session]);
  useEffect(() => {
    refresh().catch((e) => setError(errorMessage(e)));
  }, [refresh]);
  /** action runs a cluster mutation and refreshes the resulting layout and node status. */
  async function action(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  /** edit initializes the node editor from its staged or active layout assignment. */
  function edit(node: ClusterNode) {
    const staged = layout?.stagedRoleChanges.find(
      (change) => change.id === node.id && !change.remove,
    );
    const current = staged?.zone ? staged : node.role;
    setEditing(node);
    setAssignmentError("");
    setUnit("GB");
    setZone(current?.zone || "");
    setGateway(current ? current.capacity == null : false);
    setCapacity(
      current?.capacity != null ? String(current.capacity / 1e9) : "",
    );
    setTags(current?.tags?.join(", ") || "");
  }
  const pending =
    !!layout &&
    (layout.stagedRoleChanges.length > 0 || !!layout.stagedParameters);
  /** ratio formats a known count and total or a placeholder for unavailable health data. */
  const ratio = (value?: number, total?: number) =>
    value == null || total == null ? "—" : `${value} / ${total}`;
  // connectNode connects the entered Garage RPC address and refreshes cluster membership.
  function connectNode(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    void action(async () => {
      const result = await post<
        {
          /** Whether Garage completed the requested operation. */
          success: boolean;
          /** Failure message suitable for displaying beside the affected action. */
          error?: string;
        }[]
      >(AdminOperation.ConnectClusterNodes, [address.trim()]);
      if (!result[0]?.success)
        throw new Error(result[0]?.error || "Connection failed");
      setAddress("");
      setNotice("Node connected.");
    });
  }

  // stageRedundancy validates a zone-redundancy choice and stages it for the next layout version.
  function stageRedundancy(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const value =
      redundancy === "maximum" ? "maximum" : { atLeast: Number(redundancy) };
    if (
      typeof value !== "string" &&
      (!Number.isSafeInteger(value.atLeast) || value.atLeast < 1)
    ) {
      setError("enter a positive number of zones.");
      return;
    }
    void action(async () => {
      await post(AdminOperation.UpdateClusterLayout, {
        parameters: { zoneRedundancy: value },
      });
      setPreview(undefined);
      setNotice(
        "Layout settings staged. Preview and apply the layout to activate them.",
      );
    });
  }

  // applyLayout applies staged layout changes only after the user's explicit confirmation.
  async function applyLayout() {
    if (!layout) return;

    if (
      await confirm(
        "Apply all staged layout changes? This will change the cluster layout.",
      )
    )
      void action(async () => {
        await post(AdminOperation.ApplyClusterLayout, {
          version: layout.version + 1,
        });
        setPreview(undefined);
        setNotice("Layout applied.");
      });
  }

  // stageRemoval stages removal of the selected node and reports submission and refresh failures separately.
  async function stageRemoval() {
    if (!removing) return;

    setBusy(true);
    setRemoveError("");
    try {
      await post(AdminOperation.UpdateClusterLayout, {
        roles: [{ id: removing.id, remove: true }],
      });
      setRemoving(undefined);
      setPreview(undefined);
      setNotice(
        "Node removal staged. Preview and apply the layout to activate it.",
      );
      await refresh().catch((e) =>
        setError(`removal was staged, but refresh failed: ${errorMessage(e)}`),
      );
    } catch (e) {
      setRemoveError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  // stageAssignment validates node capacity and stages its storage or gateway role in the selected zone.
  function stageAssignment(e: FormEvent<HTMLFormElement>) {
    if (!editing) return;

    e.preventDefault();
    const bytes =
      Number(capacity) * ({ MB: 1e6, GB: 1e9, TB: 1e12 }[unit] || 1);
    if (!gateway && (!Number.isSafeInteger(bytes) || bytes <= 0)) {
      setAssignmentError(
        "Enter a valid positive capacity in the selected unit.",
      );
      return;
    }
    setAssignmentError("");
    setBusy(true);
    void (async () => {
      try {
        await post(AdminOperation.UpdateClusterLayout, {
          roles: [
            {
              id: editing.id,
              zone: zone.trim(),
              capacity: gateway ? null : bytes,
              tags: tags
                .split(",")
                .map((t) => t.trim())
                .filter(Boolean),
            },
          ],
        });
        setEditing(undefined);
        setPreview(undefined);
        setNotice(
          "Node assignment staged. Preview and apply the layout to activate it.",
        );
        await refresh().catch((e) =>
          setError(
            `assignment was staged, but refresh failed: ${errorMessage(e)}`,
          ),
        );
      } catch (e) {
        setAssignmentError(errorMessage(e));
      } finally {
        setBusy(false);
      }
    })();
  }

  return (
    <div className="cluster-page">
      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}
      {notice && (
        <div className="panel cluster-notice" role="status">
          {notice}
        </div>
      )}
      <div className="metrics cluster-metrics">
        {(
          [
            [
              "Connected / known nodes",
              ratio(health?.connectedNodes, health?.knownNodes),
              Network,
            ],
            [
              "Online / storage nodes",
              ratio(health?.storageNodesUp, health?.storageNodes),
              HardDrive,
            ],
            [
              "Partitions with quorum / total",
              ratio(health?.partitionsQuorum, health?.partitions),
              ShieldCheck,
            ],
            [
              "Healthy partitions / total",
              ratio(health?.partitionsAllOk, health?.partitions),
              Activity,
            ],
          ] as const
        ).map(([label, value, Icon]) => (
          <Metric
            key={label}
            label={label}
            value={value}
            icon={<Icon size={18} />}
          />
        ))}
      </div>
      <section className="panel">
        <div className="panel-head">
          <h2>Cluster details</h2>
          <button
            className="text-button"
            onClick={() => action(async () => {})}
            disabled={busy}
          >
            Refresh
          </button>
        </div>
        <dl className="cluster-details">
          <div>
            <dt>Layout version</dt>
            <dd>{status?.layoutVersion ?? "—"}</dd>
          </div>
          <div>
            <dt>Garage versions</dt>
            <dd>
              {[
                ...new Set(
                  status?.nodes.map((n) => n.garageVersion).filter(Boolean),
                ),
              ].join(", ") || "—"}
            </dd>
          </div>
        </dl>
      </section>
      <section className="panel">
        <div className="panel-head">
          <h2>Nodes</h2>
        </div>
        {allows(session, AdminOperation.ConnectClusterNodes) && (
          <form className="cluster-connect" onSubmit={connectNode}>
            <label>
              Connect a node{" "}
              <input
                required
                placeholder="node-id@host:port"
                value={address}
                onChange={(e) => setAddress(e.target.value)}
              />
            </label>
            <Button disabled={busy}>Connect</Button>
          </form>
        )}
        <div className="cluster-table-wrap">
          <table className="cluster-table">
            <thead>
              <tr>
                <th>Node</th>
                <th>Zone</th>
                <th>Capacity</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {status?.nodes.map((node) => (
                <tr key={node.id}>
                  <td>
                    <strong>{node.hostname || short(node.id)}</strong>
                    <br />
                    <code title={node.id}>{short(node.id)}</code>{" "}
                    {node.addr && <span className="muted"> · {node.addr}</span>}
                  </td>
                  <td>{node.role?.zone || "—"}</td>
                  <td>
                    {node.role
                      ? node.role.capacity == null
                        ? "Gateway"
                        : formatBytes(node.role.capacity)
                      : "Unassigned"}
                  </td>
                  <td>
                    <span
                      className={`cluster-status ${node.isUp && !node.draining ? "online" : "offline"}`}
                    >
                      {node.draining
                        ? "Draining"
                        : node.isUp
                          ? "Connected"
                          : "Disconnected"}
                    </span>
                  </td>
                  <td>
                    {allows(session, AdminOperation.UpdateClusterLayout) && (
                      <span className="cluster-node-actions">
                        <button
                          className="text-button"
                          onClick={() => edit(node)}
                        >
                          Assign
                        </button>
                        {node.role && (
                          <button
                            className="text-button"
                            disabled={busy}
                            onClick={async () => {
                              setRemoveError("");
                              setRemoving(node);
                            }}
                          >
                            Remove
                          </button>
                        )}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {status && !status.nodes.length && (
          <p className="cluster-empty">No nodes found.</p>
        )}
      </section>
      {layout?.parameters && (
        <section className="panel">
          <div className="panel-head">
            <h2>Layout settings</h2>
          </div>
          <div className="cluster-panel-body">
            <p className="layout-description">
              Zone redundancy controls the minimum number of zones holding each
              partition. Current:{" "}
              <strong>
                {layout.parameters.zoneRedundancy === "maximum"
                  ? "Maximum"
                  : `At least ${layout.parameters.zoneRedundancy.atLeast}`}
              </strong>
              {layout.stagedParameters && (
                <>
                  {" "}
                  · Staged:{" "}
                  <strong>
                    {layout.stagedParameters.zoneRedundancy === "maximum"
                      ? "Maximum"
                      : `At least ${layout.stagedParameters.zoneRedundancy.atLeast}`}
                  </strong>
                </>
              )}
            </p>
            {allows(session, AdminOperation.UpdateClusterLayout) && (
              <form className="layout-settings" onSubmit={stageRedundancy}>
                <label>
                  Zone redundancy{" "}
                  <select
                    required
                    value={redundancy}
                    onChange={(e) => setRedundancy(e.target.value)}
                  >
                    <option value="">Choose a setting</option>
                    <option value="maximum">Maximum available zones</option>
                    {Array.from(
                      {
                        length: Math.max(
                          3,
                          new Set(layout.roles?.map((r) => r.zone)).size,
                        ),
                      },
                      (_, i) => (
                        <option key={i + 1} value={i + 1}>
                          At least {i + 1} {i === 0 ? "zone" : "zones"}
                        </option>
                      ),
                    )}
                  </select>
                </label>
                <Button variant="outline" disabled={busy || !redundancy}>
                  Stage changes
                </Button>
              </form>
            )}
          </div>
        </section>
      )}
      {layout && (
        <section className="panel">
          <div className="panel-head">
            <h2>Staged layout changes</h2>
          </div>
          {pending ? (
            <div className="cluster-panel-body">
              <ul className="cluster-changes">
                {layout.stagedRoleChanges.map((c) => (
                  <li key={c.id}>
                    <code>{short(c.id)}</code>:{" "}
                    {c.remove
                      ? "remove"
                      : `${c.zone} · ${c.capacity == null ? "gateway" : formatBytes(c.capacity)} · ${c.tags?.join(", ") || "no tags"}`}
                  </li>
                ))}
                {!!layout.stagedParameters && (
                  <li>Layout parameters changed</li>
                )}
              </ul>
              {(allows(session, AdminOperation.PreviewClusterLayoutChanges) ||
                allows(session, AdminOperation.ApplyClusterLayout) ||
                allows(session, AdminOperation.RevertClusterLayout)) && (
                <div className="cluster-actions">
                  <Button
                    variant="outline"
                    disabled={
                      busy ||
                      !allows(
                        session,
                        AdminOperation.PreviewClusterLayoutChanges,
                      )
                    }
                    onClick={() =>
                      action(async () => {
                        const p = await post<LayoutPreview>(
                          AdminOperation.PreviewClusterLayoutChanges,
                        );
                        setPreview(p);
                      })
                    }
                  >
                    Preview
                  </Button>
                  <Button
                    disabled={
                      busy ||
                      !allows(session, AdminOperation.ApplyClusterLayout)
                    }
                    onClick={applyLayout}
                  >
                    Apply layout
                  </Button>
                  <Button
                    variant="outline"
                    disabled={
                      busy ||
                      !allows(session, AdminOperation.RevertClusterLayout)
                    }
                    onClick={async () => {
                      if (await confirm("Discard all staged layout changes?"))
                        void action(async () => {
                          await post(AdminOperation.RevertClusterLayout);
                          setPreview(undefined);
                        });
                    }}
                  >
                    Discard changes
                  </Button>
                </div>
              )}
              {preview && (
                <pre className="cluster-preview">
                  {preview.error ||
                    preview.message?.join("\n") ||
                    "No preview details."}
                </pre>
              )}
            </div>
          ) : (
            <p className="cluster-empty">No staged changes.</p>
          )}
        </section>
      )}
      {removing && (
        <DialogPanel
          title="Remove node from layout"
          onClose={() => {
            if (!busy) setRemoving(undefined);
          }}
        >
          <div className="cluster-remove-dialog">
            <p>
              Stage removal of{" "}
              <strong>{removing.hostname || short(removing.id)}</strong> from
              the cluster layout?
            </p>
            <p className="muted">
              The node remains in the current layout until you apply the staged
              changes.
            </p>
            {removeError && (
              <div className="alert" role="alert">
                {removeError}
              </div>
            )}
            <div className="cluster-actions">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setRemoving(undefined)}
              >
                Cancel
              </Button>
              <Button disabled={busy} onClick={stageRemoval}>
                Stage removal
              </Button>
            </div>
          </div>
        </DialogPanel>
      )}
      {editing && (
        <DialogPanel
          title="Assign node"
          onClose={() => {
            if (!busy) setEditing(undefined);
          }}
        >
          <div className="cluster-panel-body">
            <p>
              <code>{editing.id}</code>
            </p>
            {assignmentError && (
              <div className="alert" role="alert">
                {assignmentError}
              </div>
            )}
            <form className="cluster-form" onSubmit={stageAssignment}>
              <label>
                Zone
                <input
                  required
                  list="cluster-zones"
                  value={zone}
                  onChange={(e) => setZone(e.target.value)}
                />
                <datalist id="cluster-zones">
                  {[...new Set(layout?.roles?.map((r) => r.zone) || [])].map(
                    (z) => (
                      <option key={z} value={z} />
                    ),
                  )}
                </datalist>
              </label>
              <label className="cluster-checkbox">
                <input
                  type="checkbox"
                  checked={gateway}
                  onChange={(e) => setGateway(e.target.checked)}
                />{" "}
                Gateway
              </label>
              {!gateway && (
                <label>
                  Capacity
                  <span className="capacity-input">
                    <input
                      type="number"
                      min="0.000000001"
                      step="any"
                      required
                      value={capacity}
                      onChange={(e) => setCapacity(e.target.value)}
                    />
                    <select
                      aria-label="Capacity unit"
                      value={unit}
                      onChange={(e) => setUnit(e.target.value)}
                    >
                      <option>MB</option>
                      <option>GB</option>
                      <option>TB</option>
                    </select>
                  </span>
                </label>
              )}
              <label>
                Tags (comma separated)
                <input value={tags} onChange={(e) => setTags(e.target.value)} />
              </label>
              <div className="cluster-actions">
                <Button disabled={busy}>Stage assignment</Button>
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => setEditing(undefined)}
                >
                  Cancel
                </Button>
              </div>
            </form>
          </div>
        </DialogPanel>
      )}
    </div>
  );
}
