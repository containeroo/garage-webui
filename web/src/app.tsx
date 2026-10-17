import { errorMessage } from "./api";
import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  ArrowLeft,
  ChevronRight,
  CircleAlert,
  Database,
  KeyRound,
  Plus,
  RefreshCw,
  Server,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import "./style.css";
import "./theme.css";
import { UserManagement } from "./users";
import { ProfileSettings } from "./profile";
import { useTheme } from "./components/theme-picker";
import { QuestionProvider, useQuestions } from "./components/ui/questions";
import { DialogPanel } from "./components/ui/dialog";
import { Button } from "./components/ui/button";
import { UploadDialog } from "./components/upload";
import { BucketObjects } from "./components/objects";
import { BucketSettings } from "./components/bucket-settings";
import { Cluster } from "./components/cluster";
import { Usage } from "./components/usage";
import { KeySettings } from "./components/key-settings";

import {
  request,
  get,
  post,
  short,
  type Session,
  type Bucket,
  type Key,
  type Health,
  type Details,
  type Page,
} from "./api";
import { Login } from "./login";
import { formatBytes } from "./components/usage";
import { Metric, BucketRows, Empty } from "./components/storage";
import { Sidebar } from "./components/sidebar";

export function App() {
  return (
    <QuestionProvider>
      <AppContent />
    </QuestionProvider>
  );
}

function AppContent() {
  const [session, setSession] = useState<Session>();
  const [error, setError] = useState("");
  const loadSession = useCallback(() => {
    setError("");
    request<Session>("api/session")
      .then(setSession)
      .catch((e) => setError(errorMessage(e)));
  }, []);
  useEffect(() => {
    loadSession();
  }, [loadSession]);

  if (!session)
    return (
      <div className="splash">
        <div>
          <p role={error ? "alert" : undefined}>
            {error || "Connecting to Garage…"}
          </p>
          {error && (
            <button className="secondary" onClick={loadSession}>
              Try again
            </button>
          )}
        </div>
      </div>
    );
  const recoveryPage =
    window.location.pathname === new URL("login", document.baseURI).pathname;
  if (!session.user || (session.localLogin && recoveryPage))
    return <Login session={session} onLogin={setSession} />;
  return (
    <Workspace
      key={session.user}
      session={session}
      onSignedOut={() => setSession({ ...session, user: "" })}
    />
  );
}

// Workspace owns state that must not survive a change of authenticated user.
function Workspace({
  session,
  onSignedOut,
}: {
  session: Session;
  onSignedOut: () => void;
}) {
  const { confirm } = useQuestions();
  const theme = useTheme();
  const [page, setPage] = useState<Page>("overview");
  const [usageTotals, setUsageTotals] = useState<{
    bytes: number;
    objects: number;
  }>();
  const [health, setHealth] = useState<Health>();
  const [buckets, setBuckets] = useState<Bucket[]>([]);
  const [keys, setKeys] = useState<Key[]>([]);
  const [bucketsLoaded, setBucketsLoaded] = useState(false);
  const [keysLoaded, setKeysLoaded] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [modal, setModal] = useState<"bucket" | "key" | "grant" | null>(null);
  const [details, setDetails] = useState<Details>();
  const [bucketTab, setBucketTab] = useState<
    "overview" | "permissions" | "browse"
  >("overview");
  const [createdKey, setCreatedKey] = useState("");
  const [uploadBucket, setUploadBucket] = useState<Details>();
  const [objectsVersion, setObjectsVersion] = useState(0);
  const [uploadPrefix, setUploadPrefix] = useState("");
  const [editingKey, setEditingKey] = useState<string>();

  const load = useCallback(async () => {
    setBusy(true);
    setError("");
    const results = await Promise.allSettled([
      get<Health>("GetClusterHealth"),
      get<Bucket[]>("ListBuckets"),
      get<Key[]>("ListKeys"),
    ]);
    if (results[0].status === "fulfilled") setHealth(results[0].value);
    if (results[1].status === "fulfilled") setBuckets(results[1].value);
    if (results[2].status === "fulfilled") setKeys(results[2].value);
    setBucketsLoaded(results[1].status === "fulfilled");
    setKeysLoaded(results[2].status === "fulfilled");
    setLoaded(true);
    const failure = results.find((result) => result.status === "rejected");
    if (failure?.status === "rejected")
      setError(
        `Could not load all Garage data: ${errorMessage(failure.reason)}`,
      );
    setBusy(false);
  }, []);
  useEffect(() => {
    if (session?.user) void load();
  }, [session, load]);
  async function run(action: () => Promise<unknown>) {
    try {
      setError("");
      await action();
      setModal(null);
      await load();
      return true;
    } catch (e) {
      setError(errorMessage(e));
      return false;
    }
  }
  async function showBucket(
    bucket: Bucket,
    tab: "overview" | "permissions" | "browse" = "overview",
  ) {
    setUploadPrefix("");
    setBucketTab(tab);
    try {
      setDetails(
        await get<Details>(
          "GetBucketInfo",
          `?id=${encodeURIComponent(bucket.id)}`,
        ),
      );
    } catch (e) {
      setError(errorMessage(e));
    }
  }
  async function remove(kind: "Bucket" | "Key", id: string, name: string) {
    if (
      !(await confirm(
        `Delete ${kind.toLowerCase()} “${name}”? This cannot be undone.`,
      ))
    )
      return;
    const deleted = await run(() =>
      post(`Delete${kind}`, {}, `?id=${encodeURIComponent(id)}`),
    );
    if (deleted && kind === "Bucket") setDetails(undefined);
  }
  return (
    <div className="shell">
      <Sidebar
        session={session}
        page={page}
        health={health}
        onNavigate={(next) => {
          setPage(next);
          setDetails(undefined);
        }}
        onSignOut={async () => {
          try {
            await request("api/logout", { method: "POST", body: "{}" });
            onSignedOut();
          } catch (cause) {
            setError(errorMessage(cause));
          }
        }}
      />
      <main>
        <header className="topbar">
          <div>
            <span className="crumb">Workspace</span>
            <ChevronRight size={14} />
            <strong>
              {page === "overview"
                ? "Overview"
                : page === "cluster"
                  ? "Cluster"
                  : page === "buckets"
                    ? "Buckets"
                    : page === "keys"
                      ? "Access keys"
                      : page === "users"
                        ? "Users"
                        : "My profile"}
            </strong>
          </div>
          <button
            className="icon-button"
            onClick={load}
            title="Refresh"
            aria-label="Refresh"
            disabled={busy}
          >
            <RefreshCw size={17} className={busy ? "spin" : ""} />
          </button>
        </header>
        <div className="content">
          {!details && (
            <div className="page-heading">
              <div>
                <h1>
                  {page === "overview"
                    ? "Overview"
                    : page === "cluster"
                      ? "Cluster"
                      : page === "buckets"
                        ? "Buckets"
                        : page === "keys"
                          ? "Access keys"
                          : page === "users"
                            ? "Users"
                            : "My profile"}
                </h1>
              </div>
              {(page === "buckets" || page === "keys") &&
                session.role !== "viewer" && (
                  <Button
                    onClick={() =>
                      setModal(page === "buckets" ? "bucket" : "key")
                    }
                  >
                    <Plus size={17} /> Create{" "}
                    {page === "buckets" ? "bucket" : "key"}
                  </Button>
                )}
            </div>
          )}
          {error && !modal && (
            <div className="alert" role="alert">
              <CircleAlert size={18} />
              <span>{error}</span>
              <button onClick={() => setError("")}>
                <X size={16} />
              </button>
            </div>
          )}
          {!details && (
            <>
              {page === "cluster" && (
                <Cluster role={session.role} health={health} />
              )}
              {page === "overview" && (
                <>
                  <div className="metrics overview-metrics">
                    <Metric
                      icon={<Activity size={18} />}
                      label="Cluster health"
                      value={health?.status || "Unknown"}
                      highlight={health?.status === "healthy"}
                    />
                    <Metric
                      icon={<Database size={18} />}
                      label="Total usage"
                      value={usageTotals ? formatBytes(usageTotals.bytes) : "—"}
                    />
                    <Metric
                      icon={<Server size={18} />}
                      label="Total objects"
                      value={
                        usageTotals ? usageTotals.objects.toLocaleString() : "—"
                      }
                    />
                    <Metric
                      icon={<Database size={18} />}
                      label="Total buckets"
                      value={bucketsLoaded ? String(buckets.length) : "—"}
                    />
                    <Metric
                      icon={<KeyRound size={18} />}
                      label="Access keys"
                      value={keysLoaded ? String(keys.length) : "—"}
                    />
                  </div>
                  <div className="overview-grid">
                    <Usage
                      buckets={buckets}
                      loaded={bucketsLoaded}
                      busy={busy}
                      onSelect={showBucket}
                      onTotals={setUsageTotals}
                      onViewAll={() => setPage("buckets")}
                    />
                    <section className="panel">
                      <div className="panel-head">
                        <div>
                          <h2>Cluster health</h2>
                        </div>
                        <Activity size={18} className="muted" />
                      </div>
                      <div className="health-body">
                        <div className="health-summary">
                          <div className="health-icon">
                            <ShieldCheck size={23} />
                          </div>
                          <div>
                            <strong>
                              {health?.status === "healthy"
                                ? "All systems healthy"
                                : health?.status || "Status unavailable"}
                            </strong>
                          </div>
                        </div>
                        <div className="health-line">
                          <span>Storage nodes online</span>
                          <strong>
                            {health
                              ? `${health.storageNodesUp} / ${health.storageNodes}`
                              : "—"}
                          </strong>
                        </div>
                        <div className="health-line">
                          <span>Healthy partitions</span>
                          <strong>
                            {health
                              ? `${health.partitionsAllOk} / ${health.partitions}`
                              : "—"}
                          </strong>
                        </div>
                      </div>
                    </section>
                  </div>
                </>
              )}
              {page === "buckets" && (
                <section className="panel">
                  <div className="panel-head">
                    <div>
                      <h2>
                        All buckets{" "}
                        <span className="count">{buckets.length}</span>
                      </h2>
                    </div>
                  </div>
                  <BucketRows
                    buckets={buckets}
                    onSelect={showBucket}
                    loaded={bucketsLoaded}
                    loading={!loaded || busy}
                  />
                </section>
              )}
              {page === "keys" && (
                <section className="panel">
                  <div className="panel-head">
                    <div>
                      <h2>
                        All access keys{" "}
                        <span className="count">{keys.length}</span>
                      </h2>
                    </div>
                  </div>
                  {!keysLoaded ? (
                    <Empty
                      text={
                        !loaded || busy
                          ? "Loading access keys…"
                          : "Access keys unavailable"
                      }
                      detail={
                        !loaded || busy
                          ? "Please wait."
                          : "Refresh to try again."
                      }
                    />
                  ) : keys.length === 0 ? (
                    <Empty
                      text="No access keys yet"
                      detail="Create an access key to grant access to your buckets."
                    />
                  ) : (
                    <div className="table">
                      <div className="table-head key-columns">
                        <span>NAME</span>
                        <span>ACCESS KEY ID</span>
                        <span>STATUS</span>
                        <span />
                      </div>
                      {keys.map((key) => (
                        <div className="table-row key-columns" key={key.id}>
                          <div className="item-name">
                            <div className="item-icon amber">
                              <KeyRound size={16} />
                            </div>
                            <strong>{key.name || "Unnamed key"}</strong>
                          </div>
                          <code>{short(key.id)}</code>
                          <span
                            className={`badge ${key.expired ? "badge-expired" : ""}`}
                          >
                            {key.expired ? "Expired" : "Active"}
                          </span>
                          {session.role !== "viewer" && (
                            <div className="key-actions">
                              <button
                                type="button"
                                className="secondary"
                                onClick={() => setEditingKey(key.id)}
                              >
                                Edit
                              </button>
                              <button
                                className="icon-button danger"
                                title="Delete key"
                                onClick={() => remove("Key", key.id, key.name)}
                              >
                                <Trash2 size={16} />
                              </button>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                  {editingKey && (
                    <KeySettings
                      key={editingKey}
                      keyId={editingKey}
                      onChanged={() => void load()}
                      onClose={() => setEditingKey(undefined)}
                    />
                  )}
                </section>
              )}
              {page === "users" && (
                <UserManagement request={request} currentUser={session.user} />
              )}
              {page === "profile" && (
                <ProfileSettings
                  request={request}
                  localLogin={session.localLogin}
                  theme={theme}
                />
              )}
            </>
          )}
          {details && (
            <section className="panel bucket-details">
              <button
                className="text-button bucket-back"
                onClick={() => setDetails(undefined)}
              >
                <ArrowLeft size={16} /> Back to{" "}
                {page === "overview" ? "overview" : "buckets"}
              </button>
              <div className="detail-top">
                <div>
                  <h2>{details.globalAliases?.[0] || short(details.id)}</h2>
                  <code>{details.id}</code>
                </div>
              </div>
              <div
                className="bucket-tabs"
                role="tablist"
                aria-label="Bucket views"
              >
                {(
                  [
                    ["overview", "Overview"],
                    ["permissions", "Permissions"],
                    ["browse", "Browse"],
                  ] as const
                ).map(([id, label]) => (
                  <button
                    role="tab"
                    aria-selected={bucketTab === id}
                    className={bucketTab === id ? "active" : ""}
                    key={id}
                    onClick={() => setBucketTab(id)}
                  >
                    {label}
                  </button>
                ))}
              </div>
              {bucketTab === "browse" && (
                <BucketObjects
                  key={details.id}
                  bucketId={details.id}
                  canDelete={session.role !== "viewer"}
                  enabled={session.uploadsEnabled}
                  refreshVersion={objectsVersion}
                  onFolderChange={setUploadPrefix}
                  onUpload={() => setUploadBucket(details)}
                />
              )}
              {bucketTab === "overview" && (
                <BucketSettings
                  key={details.id}
                  bucket={details}
                  canEdit={session.role !== "viewer"}
                  onChanged={(bucket) => {
                    setDetails(bucket);
                    void load();
                  }}
                />
              )}
              {bucketTab === "permissions" && (
                <div className="detail-section">
                  <div className="detail-title">
                    <div>
                      <h3>Access permissions</h3>
                    </div>
                    {session.role !== "viewer" && (
                      <button
                        className="secondary"
                        onClick={() => setModal("grant")}
                      >
                        <Plus size={15} /> Add access
                      </button>
                    )}
                  </div>
                  {(details.keys || []).length === 0 ? (
                    <Empty text="No keys have access" />
                  ) : (
                    details.keys.map((grant) => (
                      <div className="grant" key={grant.accessKeyId}>
                        <div>
                          <strong>
                            {keys.find((k) => k.id === grant.accessKeyId)
                              ?.name || short(grant.accessKeyId)}
                          </strong>
                        </div>
                        <div className="grant-permissions">
                          {(["read", "write", "owner"] as const).map(
                            (permission) => (
                              <label key={permission}>
                                <input
                                  type="checkbox"
                                  checked={grant.permissions[permission]}
                                  disabled={session.role === "viewer" || busy}
                                  onChange={async (event) => {
                                    const enabled = event.target.checked;
                                    await run(async () => {
                                      await post(
                                        enabled
                                          ? "AllowBucketKey"
                                          : "DenyBucketKey",
                                        {
                                          bucketId: details.id,
                                          accessKeyId: grant.accessKeyId,
                                          permissions: {
                                            read: permission === "read",
                                            write: permission === "write",
                                            owner: permission === "owner",
                                          },
                                        },
                                      );
                                      setDetails(
                                        await get<Details>(
                                          "GetBucketInfo",
                                          `?id=${encodeURIComponent(details.id)}`,
                                        ),
                                      );
                                    });
                                  }}
                                />
                                {permission}
                              </label>
                            ),
                          )}
                        </div>
                        {session.role !== "viewer" && (
                          <button
                            className="icon-button danger"
                            title="Remove access"
                            onClick={async () => {
                              if (
                                !(await confirm(
                                  "Remove all permissions for this key?",
                                ))
                              )
                                return;
                              await run(async () => {
                                await post("DenyBucketKey", {
                                  bucketId: details.id,
                                  accessKeyId: grant.accessKeyId,
                                  permissions: {
                                    read: true,
                                    write: true,
                                    owner: true,
                                  },
                                });
                                setDetails(
                                  await get<Details>(
                                    "GetBucketInfo",
                                    `?id=${encodeURIComponent(details.id)}`,
                                  ),
                                );
                              });
                            }}
                          >
                            <Trash2 size={16} />
                          </button>
                        )}
                      </div>
                    ))
                  )}
                </div>
              )}
              {bucketTab === "overview" && session.role !== "viewer" && (
                <button
                  className="delete-bucket"
                  onClick={() =>
                    remove(
                      "Bucket",
                      details.id,
                      details.globalAliases?.[0] || details.id,
                    )
                  }
                >
                  <Trash2 size={16} /> Delete bucket
                </button>
              )}
            </section>
          )}
        </div>
      </main>
      {uploadBucket && (
        <UploadDialog
          bucketId={uploadBucket.id}
          initialPrefix={uploadPrefix}
          bucketName={uploadBucket.globalAliases?.[0] || short(uploadBucket.id)}
          onClose={() => setUploadBucket(undefined)}
          onUploaded={() => setObjectsVersion((value) => value + 1)}
        />
      )}
      {modal && (
        <DialogPanel
          title={
            modal === "bucket"
              ? "Create bucket"
              : modal === "key"
                ? "Create access key"
                : "Grant bucket access"
          }
          onClose={() => setModal(null)}
        >
          {error && (
            <div className="alert" role="alert">
              {error}
            </div>
          )}
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              const data = new FormData(e.currentTarget);
              if (modal === "bucket")
                await run(() =>
                  post("CreateBucket", { globalAlias: data.get("name") }),
                );
              if (modal === "key")
                await run(async () => {
                  const result = await post<unknown>("CreateKey", {
                    name: data.get("name"),
                  });
                  setCreatedKey(JSON.stringify(result, null, 2));
                });
              if (modal === "grant" && details) {
                await run(async () => {
                  await post("AllowBucketKey", {
                    bucketId: details.id,
                    accessKeyId: data.get("key"),
                    permissions: {
                      read: data.has("read"),
                      write: data.has("write"),
                      owner: data.has("owner"),
                    },
                  });
                  setDetails(
                    await get<Details>(
                      "GetBucketInfo",
                      `?id=${encodeURIComponent(details.id)}`,
                    ),
                  );
                });
              }
            }}
          >
            {modal !== "grant" ? (
              <label>
                Name
                <input
                  name="name"
                  required
                  autoFocus
                  placeholder={
                    modal === "bucket"
                      ? "e.g. project-backups"
                      : "e.g. application-backup"
                  }
                />
              </label>
            ) : (
              <>
                <label>
                  Access key
                  <select name="key" required>
                    {keys.map((key) => (
                      <option key={key.id} value={key.id}>
                        {key.name} · {short(key.id)}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="checks">
                  {["read", "write", "owner"].map((p) => (
                    <label key={p}>
                      <input
                        type="checkbox"
                        name={p}
                        defaultChecked={p !== "owner"}
                      />{" "}
                      {p.charAt(0).toUpperCase() + p.slice(1)}
                    </label>
                  ))}
                </div>
              </>
            )}
            <div className="modal-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() => setModal(null)}
              >
                Cancel
              </Button>
              <Button type="submit">
                {modal === "grant" ? "Grant access" : "Create"}
              </Button>
            </div>
          </form>
        </DialogPanel>
      )}
      {createdKey && (
        <DialogPanel
          title="Save your new credentials"
          onClose={() => setCreatedKey("")}
        >
          <p className="modal-note">
            Store the secret securely. It may not be shown again.
          </p>
          <pre>{createdKey}</pre>
          <div className="modal-actions">
            <button className="primary" onClick={() => setCreatedKey("")}>
              Done
            </button>
          </div>
        </DialogPanel>
      )}
    </div>
  );
}
