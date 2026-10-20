import { AdminOperation } from "../admin-operation";
import { errorMessage } from "../api";
import { useCallback, useEffect, useState } from "react";
import { Save } from "lucide-react";
import { allows, get, post, type Session } from "../api";
import { localDateTime } from "../lib/format";

/** A bucket and its effective permissions for the selected access key. */
type KeyGrants = {
  /** Stable Garage identifier for this resource. */
  id: string;
  /** Globally visible names associated with the bucket. */
  globalAliases: string[];
  /** Effective permissions granted to this credential. */
  permissions: {
    /** Whether reading bucket objects is permitted. */
    read: boolean;
    /** Whether writing bucket objects is permitted. */
    write: boolean;
    /** Whether bucket owner operations are permitted. */
    owner: boolean;
  };
};
/** An access key's editable metadata and per-bucket grants. */
type KeyDetails = {
  /** Garage S3 access-key identifier, excluding its secret. */
  accessKeyId: string;
  /** Human-readable resource label. */
  name: string;
  /** Credential expiration timestamp; absent or null means no expiration. */
  expiration?: string | null;
  /** Whether Garage considers this credential expired. */
  expired: boolean;
  /** Effective permissions granted to this credential. */
  permissions: {
    /** Whether the access key may create buckets. */
    createBucket: boolean;
  };
  /** Buckets accessible to the current credential. */
  buckets: KeyGrants[];
};

// KeySettings edits a Garage access key without ever requesting its secret.
export function KeySettings({
  keyId,
  session,
  onChanged,
  onClose,
}: {
  /** Identifier of the access key being edited. */
  keyId: string;
  /** Current credential scopes controlling key edits and individual bucket grants. */
  session: Session;
  /** Notifies the owner after authoritative resource data changes. */
  onChanged: () => void;
  /** Closes the dialog or editor owned by the parent view. */
  onClose: () => void;
}) {
  const [details, setDetails] = useState<KeyDetails>();
  const [name, setName] = useState("");
  const [expiration, setExpiration] = useState("");
  const [never, setNever] = useState(true);
  const [createBucket, setCreateBucket] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  /** refresh retrieves the current view's authoritative data before updating its state. */
  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      const result = await get<KeyDetails>(
        AdminOperation.GetKeyInfo,
        `?id=${encodeURIComponent(keyId)}`,
        { signal },
      );
      if (signal?.aborted) return;
      setDetails(result);
      setName(result.name);
      setNever(!result.expiration);
      setExpiration(result.expiration ? localDateTime(result.expiration) : "");
      setCreateBucket(result.permissions?.createBucket || false);
    },
    [keyId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void refresh(controller.signal).catch((cause) => {
      if (!controller.signal.aborted) setError(errorMessage(cause));
    });
    return () => controller.abort();
  }, [refresh]);

  /** save validates and persists access-key metadata before refreshing the editor. */
  async function save() {
    setBusy(true);
    setError("");
    try {
      if (!name.trim()) throw new Error("name is required");
      if (
        !never &&
        (!expiration || Number.isNaN(new Date(expiration).getTime()))
      )
        throw new Error("select a valid expiration date");
      const current = details?.permissions.createBucket || false;
      await post(
        AdminOperation.UpdateKey,
        {
          name: name.trim(),
          ...(never
            ? { neverExpires: true }
            : { expiration: new Date(expiration).toISOString() }),
          ...(current === createBucket
            ? {}
            : { [createBucket ? "allow" : "deny"]: { createBucket: true } }),
        },
        `?id=${encodeURIComponent(keyId)}`,
      );
      await refresh();
      onChanged();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  /** updatePermissions changes a single bucket permission for the selected access key. */
  async function updatePermissions(
    bucket: KeyGrants,
    permission: "read" | "write" | "owner",
    enabled: boolean,
  ) {
    setBusy(true);
    setError("");
    try {
      await post(
        enabled ? AdminOperation.AllowBucketKey : AdminOperation.DenyBucketKey,
        {
          bucketId: bucket.id,
          accessKeyId: keyId,
          permissions: { [permission]: true },
        },
      );
      await refresh();
      onChanged();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="key-editor">
      <div className="editor-heading">
        <div>
          <h3>Edit access key</h3>
          <p>Update key details and bucket-specific permissions.</p>
        </div>
        <button type="button" className="secondary" onClick={onClose}>
          Close
        </button>
      </div>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      {!details ? (
        <p className="editor-loading" role="status">
          Loading key details…
        </p>
      ) : (
        <>
          <div className="editor-section">
            <div className="editor-section-heading">
              <h4>Key details</h4>
              <code title={details.accessKeyId}>{details.accessKeyId}</code>
            </div>
            <div className="form-grid">
              <label>
                Name
                <input
                  value={name}
                  disabled={busy || !allows(session, AdminOperation.UpdateKey)}
                  onChange={(event) => setName(event.target.value)}
                />
              </label>
              <div className="field-stack">
                <span className="field-label">Expiration</span>
                <span className="field-hint">
                  Set a date or allow the key to remain active.
                </span>
                <label className="inline-check">
                  <input
                    type="checkbox"
                    checked={never}
                    disabled={
                      busy || !allows(session, AdminOperation.UpdateKey)
                    }
                    onChange={(event) => setNever(event.target.checked)}
                  />
                  Never expires
                </label>
                {!never && (
                  <input
                    aria-label="Expiration (local time)"
                    type="datetime-local"
                    value={expiration}
                    disabled={
                      busy || !allows(session, AdminOperation.UpdateKey)
                    }
                    onChange={(event) => setExpiration(event.target.value)}
                  />
                )}
              </div>
            </div>
            <label className="permission-toggle">
              <input
                type="checkbox"
                checked={createBucket}
                disabled={busy || !allows(session, AdminOperation.UpdateKey)}
                onChange={(event) => setCreateBucket(event.target.checked)}
              />
              <span>
                <strong>Allow creating buckets</strong>
                <small>Permit this access key to create new buckets.</small>
              </span>
            </label>
            <div className="editor-actions">
              <button
                type="button"
                className="primary"
                disabled={
                  busy ||
                  !name.trim() ||
                  !allows(session, AdminOperation.UpdateKey)
                }
                onClick={() => void save()}
              >
                <Save size={15} /> {busy ? "Saving…" : "Save key"}
              </button>
            </div>
          </div>

          <div className="editor-section">
            <div className="editor-section-heading">
              <div>
                <h4>Bucket permissions</h4>
                <p>Changes to these permissions take effect immediately.</p>
              </div>
            </div>
            {details.buckets?.length ? (
              <div className="permission-table">
                <div className="permission-header" aria-hidden="true">
                  <span>Bucket</span>
                  <span>Read</span>
                  <span>Write</span>
                  <span>Owner</span>
                </div>
                {details.buckets.map((bucket) => (
                  <div className="permission-row" key={bucket.id}>
                    <div
                      className="permission-bucket"
                      title={bucket.globalAliases?.[0] || bucket.id}
                    >
                      <strong>{bucket.globalAliases?.[0] || bucket.id}</strong>
                      <small>{bucket.id}</small>
                    </div>
                    {(["read", "write", "owner"] as const).map((permission) => (
                      <label className="permission-cell" key={permission}>
                        <input
                          type="checkbox"
                          checked={bucket.permissions[permission]}
                          disabled={
                            busy ||
                            !allows(
                              session,
                              bucket.permissions[permission]
                                ? AdminOperation.DenyBucketKey
                                : AdminOperation.AllowBucketKey,
                            )
                          }
                          aria-label={`${permission} ${bucket.globalAliases?.[0] || bucket.id}`}
                          onChange={(event) =>
                            void updatePermissions(
                              bucket,
                              permission,
                              event.target.checked,
                            )
                          }
                        />
                        <span className="mobile-permission-label">
                          {permission}
                        </span>
                      </label>
                    ))}
                  </div>
                ))}
              </div>
            ) : (
              <p className="editor-empty">
                No bucket permissions yet. Grant access from a bucket's details.
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
