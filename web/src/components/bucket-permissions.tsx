import type { Grant } from "../api";
import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import {
  allows,
  errorMessage,
  get,
  post,
  short,
  type BucketDetails,
  type AccessKey,
  type Session,
} from "../api";
import { Empty } from "./storage";
import { useQuestions } from "./ui/questions";

// BucketPermissions manages grants independently of workspace navigation and loading.
export function BucketPermissions({
  bucket,
  keys,
  session,
  onChanged,
  onAddAccess,
}: {
  /** Current bucket details supplied by the owning workspace. */
  bucket: BucketDetails;
  /** Access-key summaries or grants available to the current view. */
  keys: AccessKey[];
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
  /** Notifies the owner after authoritative resource data changes. */
  onChanged: (bucket: BucketDetails) => void;
  /** Opens the workspace dialog for granting a key access. */
  onAddAccess: () => void;
}) {
  const { confirm } = useQuestions();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const keyNames = new Map(keys.map((key) => [key.id, key.name]));

  // run displays permission failures beside the affected grants.
  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await action();
      onChanged(
        await get<BucketDetails>(
          "GetBucketInfo",
          `?id=${encodeURIComponent(bucket.id)}`,
        ),
      );
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  // updateGrant changes one key permission and refreshes the selected bucket's grants.
  async function updateGrant(
    grant: Grant,
    permission: keyof Grant["permissions"],
    enabled: boolean,
  ) {
    await run(async () => {
      await post(enabled ? "AllowBucketKey" : "DenyBucketKey", {
        bucketId: bucket.id,
        accessKeyId: grant.accessKeyId,
        permissions: {
          read: permission === "read",
          write: permission === "write",
          owner: permission === "owner",
        },
      });
    });
  }

  // revokeGrant confirms removal of all bucket permissions for the selected access key.
  async function revokeGrant(grant: Grant) {
    if (!(await confirm("Remove all permissions for this key?"))) return;
    await run(async () => {
      await post("DenyBucketKey", {
        bucketId: bucket.id,
        accessKeyId: grant.accessKeyId,
        permissions: {
          read: true,
          write: true,
          owner: true,
        },
      });
    });
  }

  return (
    <>
      {error && (
        <p role="alert" className="object-error">
          {error}
        </p>
      )}
      <div className="detail-section">
        <div className="detail-title">
          <div>
            <h3>Access permissions</h3>
          </div>
          {allows(session, "AllowBucketKey") && (
            <button className="secondary" disabled={busy} onClick={onAddAccess}>
              <Plus size={15} /> Add access
            </button>
          )}
        </div>
        {(bucket.keys || []).length === 0 ? (
          <Empty text="No keys have access" />
        ) : (
          bucket.keys.map((grant) => (
            <div className="grant" key={grant.accessKeyId}>
              <div className="grant-identity">
                <strong
                  title={keyNames.get(grant.accessKeyId) || grant.accessKeyId}
                >
                  {keyNames.get(grant.accessKeyId) || short(grant.accessKeyId)}
                </strong>
                <small title={grant.accessKeyId}>{grant.accessKeyId}</small>
              </div>
              <div className="grant-permissions">
                {(["read", "write", "owner"] as const).map((permission) => (
                  <label key={permission}>
                    <input
                      type="checkbox"
                      aria-label={`${permission} permission for ${grant.accessKeyId}`}
                      checked={grant.permissions[permission]}
                      disabled={
                        !allows(
                          session,
                          grant.permissions[permission]
                            ? "DenyBucketKey"
                            : "AllowBucketKey",
                        ) || busy
                      }
                      onChange={(event) =>
                        void updateGrant(
                          grant,
                          permission,
                          event.target.checked,
                        )
                      }
                    />
                    {permission}
                  </label>
                ))}
              </div>
              {allows(session, "DenyBucketKey") && (
                <button
                  className="icon-button danger"
                  title="Remove access"
                  disabled={busy}
                  onClick={() => void revokeGrant(grant)}
                >
                  <Trash2 size={16} />
                </button>
              )}
            </div>
          ))
        )}
      </div>
    </>
  );
}
