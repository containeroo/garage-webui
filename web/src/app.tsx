import { AdminOperation } from "./admin-operation";
import { BucketTab, CredentialKind } from "./domain";
import type { FormEvent } from "react";
import { errorMessage } from "./api";
import { useCallback, useEffect, useRef, useState } from "react";
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
import { AdminTokens } from "./components/admin-tokens";
import { ProfileSettings } from "./profile";
import { useLocalTheme, useTheme } from "./components/theme-picker";
import { QuestionProvider, useQuestions } from "./components/ui/questions";
import { DialogPanel } from "./components/ui/dialog";
import { Button } from "./components/ui/button";
import { UploadDialog } from "./components/upload";
import { BucketObjects } from "./components/objects";
import { BucketSettings } from "./components/bucket-settings";
import { BucketPermissions } from "./components/bucket-permissions";
import { Cluster } from "./components/cluster";
import { Usage } from "./components/usage";
import { KeySettings } from "./components/key-settings";

import {
  allows,
  request,
  get,
  post,
  short,
  type Session,
  type Bucket,
  type BucketDetails,
  type WorkspacePage,
} from "./api";
import { Login } from "./login";
import { formatBytes } from "./lib/format";
import { Metric, BucketRows, Empty } from "./components/storage";
import { Sidebar } from "./components/sidebar";
import { useWorkspaceData } from "./hooks/use-workspace-data";
import { pageTitle } from "./lib/navigation";

/** App supplies the shared dialog context around the authenticated console. */
export function App() {
  return (
    <QuestionProvider>
      <AppContent />
    </QuestionProvider>
  );
}

/** AppContent loads the session and chooses sign-in or the authenticated workspace. */
function AppContent() {
  useLocalTheme();
  const [session, setSession] = useState<Session>();
  const [error, setError] = useState("");
  /** loadSession retrieves the current credential metadata and exposes connection failures. */
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
  if (!session.user) return <Login onLogin={setSession} />;
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
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
  /** Clears authenticated workspace state after session revocation. */
  onSignedOut: () => void;
}) {
  const { confirm } = useQuestions();
  const theme = useTheme();
  const [page, setPage] = useState<WorkspacePage>(
    session.mode === CredentialKind.Admin ? "overview" : "buckets",
  );
  const [usageTotals, setUsageTotals] = useState<{
    /** Total stored object bytes. */
    bytes: number;
    /** Total number of stored objects. */
    objects: number;
  }>();
  const {
    health,
    buckets,
    keys,
    bucketsLoaded,
    keysLoaded,
    loaded,
    busy: loading,
    error,
    setError,
    load,
  } = useWorkspaceData(session);
  const [mutating, setMutating] = useState(false);
  const mutationPending = useRef(false);
  const bucketRequest = useRef<AbortController | null>(null);
  const busy = loading || mutating;
  const [modal, setModal] = useState<"bucket" | "key" | "grant" | null>(null);
  const [details, setDetails] = useState<BucketDetails>();
  const [bucketTab, setBucketTab] = useState<BucketTab>(BucketTab.Overview);
  const [createdKey, setCreatedKey] = useState("");
  const [uploadBucket, setUploadBucket] = useState<BucketDetails>();
  const [objectsVersion, setObjectsVersion] = useState(0);
  const [uploadPrefix, setUploadPrefix] = useState("");
  const [editingKey, setEditingKey] = useState<string>();

  useEffect(() => () => bucketRequest.current?.abort(), []);

  // closeBucket cancels pending detail loads before returning to workspace navigation.
  function closeBucket() {
    bucketRequest.current?.abort();
    setDetails(undefined);
  }

  /** run executes a mutation and presents its failure in the owning view. */
  async function run(action: () => Promise<unknown>) {
    if (mutationPending.current) return false;
    mutationPending.current = true;
    setMutating(true);
    try {
      setError("");
      await action();
      setModal(null);
      await load();
      return true;
    } catch (e) {
      setError(errorMessage(e));
      return false;
    } finally {
      mutationPending.current = false;
      setMutating(false);
    }
  }
  /** showBucket loads the latest selected bucket and ignores cancelled detail requests. */
  async function showBucket(
    bucket: Bucket,
    tab: BucketTab = BucketTab.Overview,
  ) {
    bucketRequest.current?.abort();
    const controller = new AbortController();
    bucketRequest.current = controller;
    setUploadPrefix("");
    setBucketTab(session.mode === CredentialKind.S3 ? BucketTab.Browse : tab);
    try {
      const bucketDetails = await get<BucketDetails>(
        AdminOperation.GetBucketInfo,
        `?id=${encodeURIComponent(bucket.id)}`,
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) setDetails(bucketDetails);
    } catch (e) {
      if (!controller.signal.aborted) setError(errorMessage(e));
    }
  }
  /** remove confirms deletion and reports failures before refreshing the affected view. */
  async function remove(kind: "Bucket" | "Key", id: string, name: string) {
    if (
      !(await confirm(
        `Delete ${kind.toLowerCase()} “${name}”? This cannot be undone.`,
      ))
    )
      return;
    const deleted = await run(() =>
      post(
        kind === "Bucket"
          ? AdminOperation.DeleteBucket
          : AdminOperation.DeleteKey,
        {},
        `?id=${encodeURIComponent(id)}`,
      ),
    );
    if (deleted && kind === "Bucket") setDetails(undefined);
  }
  // submitResource creates a bucket or key, or grants bucket access, using the active workspace dialog.
  async function submitResource(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    if (modal === "bucket")
      await run(() =>
        post(AdminOperation.CreateBucket, { globalAlias: data.get("name") }),
      );
    if (modal === "key")
      await run(async () => {
        const result = await post<unknown>(AdminOperation.CreateKey, {
          name: data.get("name"),
        });
        setCreatedKey(JSON.stringify(result, null, 2));
      });
    if (modal === "grant" && details) {
      await run(async () => {
        await post(AdminOperation.AllowBucketKey, {
          bucketId: details.id,
          accessKeyId: data.get("key"),
          permissions: {
            read: data.has("read"),
            write: data.has("write"),
            owner: data.has("owner"),
          },
        });
        setDetails(
          await get<BucketDetails>(
            AdminOperation.GetBucketInfo,
            `?id=${encodeURIComponent(details.id)}`,
          ),
        );
      });
    }
  }

  return (
    <div className="shell">
      <Sidebar
        session={session}
        page={page}
        health={health}
        onNavigate={(next) => {
          setPage(next);
          closeBucket();
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
            <strong>{pageTitle(page)}</strong>
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
                <h1>{pageTitle(page)}</h1>
              </div>
              {(page === "buckets" || page === "keys") &&
                allows(
                  session,
                  page === "keys"
                    ? AdminOperation.CreateKey
                    : AdminOperation.CreateBucket,
                ) && (
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
                <Cluster session={session} health={health} />
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
                    browseOnly={session.mode === CredentialKind.S3}
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
                          {session.mode === CredentialKind.Admin && (
                            <div className="key-actions">
                              <button
                                type="button"
                                className="secondary"
                                disabled={
                                  !allows(session, AdminOperation.GetKeyInfo)
                                }
                                onClick={() => setEditingKey(key.id)}
                              >
                                Edit
                              </button>
                              <button
                                className="icon-button danger"
                                disabled={
                                  !allows(session, AdminOperation.DeleteKey)
                                }
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
                      session={session}
                      onChanged={() => void load()}
                      onClose={() => setEditingKey(undefined)}
                    />
                  )}
                </section>
              )}
              {page === "tokens" && <AdminTokens session={session} />}
              {page === "profile" && (
                <ProfileSettings session={session} theme={theme} />
              )}
            </>
          )}
          {details && (
            <section className="panel bucket-details">
              <button className="text-button bucket-back" onClick={closeBucket}>
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
                    [BucketTab.Overview, "Overview"],
                    [BucketTab.Permissions, "Permissions"],
                    [BucketTab.Browse, "Browse"],
                  ] as const
                )
                  .filter(
                    ([id]) =>
                      session.mode === CredentialKind.Admin ||
                      id === BucketTab.Browse,
                  )
                  .map(([id, label]) => (
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
              {bucketTab === BucketTab.Browse && (
                <BucketObjects
                  key={details.id}
                  bucketId={details.id}
                  canDelete={true}
                  enabled={session.uploadsEnabled}
                  refreshVersion={objectsVersion}
                  onFolderChange={setUploadPrefix}
                  onUpload={() => setUploadBucket(details)}
                />
              )}
              {bucketTab === BucketTab.Overview && (
                <BucketSettings
                  key={details.id}
                  bucket={details}
                  session={session}
                  onChanged={(bucket) => {
                    setDetails(bucket);
                    void load();
                  }}
                />
              )}
              {bucketTab === BucketTab.Permissions && (
                <BucketPermissions
                  key={details.id}
                  bucket={details}
                  keys={keys}
                  session={session}
                  onChanged={setDetails}
                  onAddAccess={() => setModal("grant")}
                />
              )}
              {bucketTab === BucketTab.Overview &&
                allows(session, AdminOperation.DeleteBucket) && (
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
          <form onSubmit={submitResource}>
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
              <Button type="submit" disabled={busy}>
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
