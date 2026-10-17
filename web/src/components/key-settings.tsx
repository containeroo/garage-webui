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
      if (!name.trim()) throw new Error("Name is required");
      if (
        !never &&
        (!expiration || Number.isNaN(new Date(expiration).getTime()))
      )
        throw new Error("Select a valid expiration date");
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
      <div className="object-toolbar">
        <strong>Edit access key</strong>
        <button className="secondary" onClick={onClose}>
          Close
        </button>
      </div>
      {error && (
        <p className="object-error" role="alert">
          {error}
        </p>
      )}
      {!details ? (
        <p>Loading…</p>
      ) : (
        <>
          <code>{details.accessKeyId}</code>
          <label>
            Name
            <input
              value={name}
              disabled={busy}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label className="setting-toggle">
            <input
              type="checkbox"
              checked={never}
              disabled={busy}
              onChange={(event) => setNever(event.target.checked)}
            />{" "}
            Never expires
          </label>
          {!never && (
            <label>
              Expiration (local time)
              <input
                type="datetime-local"
                value={expiration}
                disabled={busy}
                onChange={(event) => setExpiration(event.target.value)}
              />
            </label>
          )}
          <label className="setting-toggle">
            <input
              type="checkbox"
              checked={createBucket}
              disabled={busy}
              onChange={(event) => setCreateBucket(event.target.checked)}
            />{" "}
            Allow creating buckets
          </label>
          <button
            className="secondary"
            disabled={busy}
            onClick={() => void save()}
          >
            <Save size={15} /> Save key
          </button>
          <h4>Bucket permissions</h4>
          {details.buckets?.length ? (
            details.buckets.map((bucket) => (
              <div className="bucket-setting-group" key={bucket.id}>
                <strong>{bucket.globalAliases?.[0] || bucket.id}</strong>
                {(["read", "write", "owner"] as const).map((permission) => (
                  <label className="setting-toggle" key={permission}>
                    <input
                      type="checkbox"
                      checked={bucket.permissions[permission]}
                      disabled={busy}
                      onChange={(event) =>
                        void updatePermissions(
                          bucket,
                          permission,
                          event.target.checked,
                        )
                      }
                    />
                    {permission}
                  </label>
                ))}
              </div>
            ))
          ) : (
            <small>
              No bucket permissions. Add access from a bucket's details.
            </small>
          )}
        </>
      )}
    </div>
  );
}
