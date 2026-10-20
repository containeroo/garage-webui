import type { FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import {
  Plus,
  Copy,
  Trash2,
  Pencil,
  Search,
  ShieldCheck,
  KeyRound,
} from "lucide-react";
import { allows, errorMessage, get, post, type Session } from "../api";
import { DialogPanel } from "./ui/dialog";
import { useQuestions } from "./ui/questions";
/** A named or deployment-managed Garage admin token without its secret. */
type Token = {
  /** Stable Garage identifier for this resource. */
  id: string | null;
  /** Human-readable resource label. */
  name: string;
  /** Credential expiration timestamp; absent or null means no expiration. */
  expiration: string | null;
  /** Whether Garage considers this credential expired. */
  expired: boolean;
  /** Garage operations permitted by the admin token. */
  scope: string[];
};
const highPrivilegeScopes = ["CreateAdminToken", "UpdateAdminToken"];

// grantsAdministration identifies scopes that can delegate full administrative access.
function grantsAdministration(scope: string[]): boolean {
  return (
    scope.includes("*") ||
    scope.some((operation) => highPrivilegeScopes.includes(operation))
  );
}

const operations = [
  "AddBucketAlias",
  "AllowBucketKey",
  "ApplyClusterLayout",
  "CheckDomain",
  "CleanupIncompleteUploads",
  "ClusterLayoutSkipDeadNodes",
  "ConnectClusterNodes",
  "CreateAdminToken",
  "CreateBucket",
  "CreateKey",
  "CreateMetadataSnapshot",
  "DeleteAdminToken",
  "DeleteBucket",
  "DeleteKey",
  "DenyBucketKey",
  "GetAdminTokenInfo",
  "GetBlockInfo",
  "GetBucketInfo",
  "GetClusterHealth",
  "GetClusterLayout",
  "GetClusterLayoutHistory",
  "GetClusterStatistics",
  "GetClusterStatus",
  "GetCurrentAdminTokenInfo",
  "GetKeyInfo",
  "GetNodeInfo",
  "GetNodeStatistics",
  "GetWorkerInfo",
  "GetWorkerVariable",
  "Health",
  "ImportKey",
  "InspectObject",
  "LaunchRepairOperation",
  "ListAdminTokens",
  "ListBlockErrors",
  "ListBuckets",
  "ListBucketsInfo",
  "ListKeys",
  "ListKeysInfo",
  "ListWorkers",
  "Metrics",
  "PreviewClusterLayoutChanges",
  "PurgeBlocks",
  "RemoveBucketAlias",
  "RetryBlockResync",
  "RevertClusterLayout",
  "SetWorkerVariable",
  "UpdateAdminToken",
  "UpdateBucket",
  "UpdateClusterLayout",
  "UpdateKey",
];
/** AdminTokens lists, creates, edits, and revokes Garage administrative credentials. */
export function AdminTokens({
  session,
}: {
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
}) {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Token | null | undefined>();
  const [name, setName] = useState("");
  const [expiration, setExpiration] = useState("");
  const [scope, setScope] = useState<string[]>([]);
  const [scopeFilter, setScopeFilter] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const { confirm } = useQuestions();
  /** load retrieves the current view's data and reports unavailable results. */
  const load = useCallback(
    (signal?: AbortSignal) =>
      get<Token[]>("ListAdminTokens", "", { signal })
        .then((result) => {
          if (signal?.aborted) return;
          setTokens(result);
          setError("");
        })
        .catch((e) => {
          if (!signal?.aborted) setError(errorMessage(e));
        })
        .finally(() => {
          if (!signal?.aborted) setLoading(false);
        }),
    [],
  );
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  /** open initializes the admin-token editor from an existing token or empty defaults. */
  function open(token: Token | null) {
    setEditing(token);
    setName(token?.name || "");
    setExpiration(
      token?.expiration
        ? new Date(token.expiration).toISOString().slice(0, 16)
        : "",
    );
    setScope(token?.scope || []);
    setScopeFilter("");
    setError("");
  }
  const visibleOperations = operations.filter((operation) =>
    operation.toLowerCase().includes(scopeFilter.toLowerCase().trim()),
  );

  // revokeToken confirms token revocation and refreshes the administrative credential list.
  async function revokeToken(token: Token) {
    if (!token.id) return;
    if (
      !(await confirm(
        `Revoke token “${token.name}”? Its sessions will lose access.`,
      ))
    )
      return;

    try {
      await post("DeleteAdminToken", {}, `?id=${encodeURIComponent(token.id)}`);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  // submitToken validates privileged scopes before saving the admin token and revealing any new secret.
  async function submitToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (editing === undefined || (editing && !editing.id) || busy) return;
    setBusy(true);
    setError("");

    try {
      if (grantsAdministration(scope)) {
        const accepted = await confirm(
          "This token can grant full Garage administration permissions. Continue?",
        );
        if (!accepted) return;
      }

      const result = await post<
        Token & {
          /** New admin token returned only when Garage provides its secret. */
          secretToken?: string;
        }
      >(
        editing ? "UpdateAdminToken" : "CreateAdminToken",
        {
          name: name.trim(),
          scope,
          neverExpires: !expiration,
          ...(expiration
            ? { expiration: new Date(expiration + "Z").toISOString() }
            : {}),
        },
        editing?.id ? `?id=${encodeURIComponent(editing.id)}` : "",
      );
      setSecret(result.secretToken || "");
      setEditing(undefined);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <section className="panel settings-panel">
        <div className="panel-head">
          <div>
            <h2>
              Admin tokens <span className="count">{tokens.length}</span>
            </h2>
            <p>Manage Garage Admin API access and token expiration.</p>
          </div>
          {allows(session, "CreateAdminToken") && (
            <button className="primary" onClick={() => open(null)}>
              <Plus size={16} /> Create token
            </button>
          )}
        </div>
        {error && editing === undefined && (
          <p role="alert" className="alert inline-alert">
            {error}
          </p>
        )}
        {loading ? (
          <div className="empty" role="status">
            Loading admin tokens…
          </div>
        ) : error && !tokens.length ? null : tokens.length === 0 ? (
          <div className="empty">
            <ShieldCheck size={24} />
            <strong>No admin tokens found</strong>
            <span>Create a named token to delegate administrative access.</span>
          </div>
        ) : (
          <div className="table token-table">
            <div className="table-head token-columns">
              <span>Name</span>
              <span>Allowed operations</span>
              <span>Expiration</span>
              <span className="actions-heading">Actions</span>
            </div>
            {tokens.map((token) => (
              <div
                className="table-row token-columns"
                key={token.id || token.name}
              >
                <div className="token-identity">
                  <span className="item-icon">
                    <KeyRound size={16} />
                  </span>
                  <div className="token-identity-text">
                    <strong title={token.name}>{token.name}</strong>
                    <small title={token.id || undefined}>
                      {token.id || "Deployment token"}
                    </small>
                  </div>
                </div>
                <div className="token-summary" title={token.scope.join(", ")}>
                  {token.scope.includes("*") ? (
                    <span className="status-pill">All operations</span>
                  ) : (
                    <>
                      <span className="status-pill">
                        {token.scope.length} operations
                      </span>
                      <span className="token-scope-preview">
                        {token.scope.slice(0, 2).join(", ")}
                        {token.scope.length > 2 ? "…" : ""}
                      </span>
                    </>
                  )}
                </div>
                <span
                  className={
                    token.expired
                      ? "status-pill is-expired"
                      : "token-expiration"
                  }
                >
                  {token.expired
                    ? "Expired"
                    : token.expiration
                      ? new Date(token.expiration).toLocaleDateString()
                      : "Never"}
                </span>
                <div className="key-actions">
                  {token.id && allows(session, "UpdateAdminToken") && (
                    <button
                      title="Edit token"
                      aria-label={`Edit ${token.name}`}
                      className="icon-button"
                      onClick={() => open(token)}
                    >
                      <Pencil size={16} />
                    </button>
                  )}
                  {token.id && allows(session, "DeleteAdminToken") && (
                    <button
                      title="Revoke token"
                      aria-label={`Revoke ${token.name}`}
                      className="icon-button danger"
                      onClick={() => void revokeToken(token)}
                    >
                      <Trash2 size={16} />
                    </button>
                  )}
                  {!token.id && (
                    <span
                      className="muted"
                      title="Managed by Garage configuration"
                    >
                      —
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {editing !== undefined && (
        <DialogPanel
          className="token-dialog"
          title={editing ? "Edit admin token" : "Create admin token"}
          onClose={() => setEditing(undefined)}
        >
          <form className="token-form" onSubmit={submitToken}>
            <div className="token-form-scroll">
              {error && (
                <p className="alert" role="alert">
                  {error}
                </p>
              )}
              <p className="form-intro">
                Choose a descriptive name and grant only the operations this
                token needs.
              </p>

              <div className="form-grid">
                <label>
                  Name
                  <input
                    required
                    autoFocus
                    value={name}
                    disabled={busy}
                    placeholder="e.g. backup-operator"
                    onChange={(event) => setName(event.target.value)}
                  />
                </label>
                <label>
                  Expires at (UTC)
                  <input
                    type="datetime-local"
                    value={expiration}
                    disabled={busy}
                    onChange={(event) => setExpiration(event.target.value)}
                  />
                  <small className="field-hint">
                    Leave blank to never expire.
                  </small>
                </label>
              </div>

              <fieldset className="scope-fieldset">
                <legend className="sr-only">Allowed operations</legend>
                <div className="scope-heading">
                  <div>
                    <h3>Allowed operations</h3>
                    <p>Grant the minimum permissions needed.</p>
                  </div>
                  <span className="status-pill">
                    {scope.includes("*") ? "All" : `${scope.length} selected`}
                  </span>
                </div>
                <label className="scope-all">
                  <input
                    type="checkbox"
                    checked={scope.includes("*")}
                    disabled={busy}
                    onChange={(event) =>
                      setScope(event.target.checked ? ["*"] : [])
                    }
                  />
                  <span>
                    <strong>All operations</strong>
                    <small>Unrestricted Garage administration access</small>
                  </span>
                </label>
                {!scope.includes("*") && (
                  <>
                    <div className="scope-search">
                      <Search size={17} aria-hidden="true" />
                      <input
                        aria-label="Filter operations"
                        placeholder="Filter operations…"
                        value={scopeFilter}
                        onChange={(event) => setScopeFilter(event.target.value)}
                      />
                    </div>
                    <div
                      className="token-scopes"
                      role="group"
                      aria-label="Individual operations"
                    >
                      {visibleOperations.map((operation) => (
                        <label className="scope-option" key={operation}>
                          <input
                            type="checkbox"
                            checked={scope.includes(operation)}
                            disabled={busy}
                            onChange={(event) =>
                              setScope((current) =>
                                event.target.checked
                                  ? [...current, operation]
                                  : current.filter(
                                      (value) => value !== operation,
                                    ),
                              )
                            }
                          />
                          <span title={operation}>{operation}</span>
                        </label>
                      ))}
                      {visibleOperations.length === 0 && (
                        <p className="scope-empty">No matching operations</p>
                      )}
                    </div>
                  </>
                )}
                <p className="scope-warning">
                  CreateAdminToken and UpdateAdminToken can grant full
                  administration access, even when other operations are
                  restricted.
                </p>
              </fieldset>
            </div>
            <div className="modal-actions token-form-actions">
              <button
                type="button"
                className="secondary"
                disabled={busy}
                onClick={() => setEditing(undefined)}
              >
                Cancel
              </button>
              <button
                className="primary"
                disabled={busy || !name.trim() || !scope.length}
              >
                {busy ? "Saving…" : editing ? "Save token" : "Create token"}
              </button>
            </div>
          </form>
        </DialogPanel>
      )}

      {secret && (
        <DialogPanel
          title="Save your admin token"
          onClose={() => setSecret("")}
        >
          <div className="secret-panel">
            {error && (
              <p role="alert" className="object-error">
                {error}
              </p>
            )}
            <p className="form-intro">
              This token is shown only once. Copy it and store it securely
              before closing.
            </p>
            <label>
              Secret token
              <textarea
                readOnly
                value={secret}
                onFocus={(event) => event.target.select()}
              />
            </label>
            <button
              className="secondary"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(secret);
                } catch {
                  setError("copy failed. Select and copy the token manually.");
                }
              }}
            >
              <Copy size={16} /> Copy token
            </button>
            <div className="modal-actions">
              <button className="primary" onClick={() => setSecret("")}>
                Done
              </button>
            </div>
          </div>
        </DialogPanel>
      )}
    </>
  );
}
