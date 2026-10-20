import { errorMessage } from "../api";
import { useEffect, useState } from "react";
import { Save } from "lucide-react";
import { get, post } from "../api";

type KeyGrants = {
  id: string;
  globalAliases: string[];
  permissions: { read: boolean; write: boolean; owner: boolean };
};
type KeyDetails = {
  accessKeyId: string;
  name: string;
  expiration?: string | null;
  expired: boolean;
  permissions: { createBucket: boolean };
  buckets: KeyGrants[];
};

// KeySettings edits a Garage access key without ever requesting its secret.
export function KeySettings({
  keyId,
  onChanged,
  onClose,
}: {
  keyId: string;
  onChanged: () => void;
  onClose: () => void;
}) {
  const [details, setDetails] = useState<KeyDetails>();
  const [name, setName] = useState("");
  const [expiration, setExpiration] = useState("");
  const [never, setNever] = useState(true);
  const [createBucket, setCreateBucket] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function refresh() {
    const result = await get<KeyDetails>(
      "GetKeyInfo",
      `?id=${encodeURIComponent(keyId)}`,
    );
    setDetails(result);
    setName(result.name);
    setNever(!result.expiration);
    setExpiration(result.expiration ? result.expiration.slice(0, 16) : "");
    setCreateBucket(result.permissions?.createBucket || false);
  }

  useEffect(() => {
    void refresh().catch((cause) => setError(errorMessage(cause)));
  }, [keyId]);

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
        "UpdateKey",
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

  async function updatePermissions(
    bucket: KeyGrants,
    permission: "read" | "write" | "owner",
    enabled: boolean,
  ) {
    setBusy(true);
    setError("");
    try {
      await post(enabled ? "AllowBucketKey" : "DenyBucketKey", {
        bucketId: bucket.id,
        accessKeyId: keyId,
        permissions: { [permission]: true },
      });
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
                  disabled={busy}
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
                    disabled={busy}
                    onChange={(event) => setNever(event.target.checked)}
                  />
                  Never expires
                </label>
                {!never && (
                  <input
                    aria-label="Expiration (local time)"
                    type="datetime-local"
                    value={expiration}
                    disabled={busy}
                    onChange={(event) => setExpiration(event.target.value)}
                  />
                )}
              </div>
            </div>
            <label className="permission-toggle">
              <input
                type="checkbox"
                checked={createBucket}
                disabled={busy}
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
                disabled={busy || !name.trim()}
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
                          disabled={busy}
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
