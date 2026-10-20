import { useQuestions } from "./ui/questions";
import { errorMessage } from "../api";
import { useState } from "react";
import { Plus, Save, Trash2 } from "lucide-react";
import {
  allows,
  get,
  post,
  type BucketDetails,
  type LifecycleRule,
  type Session,
} from "../api";
import { formatBinaryBytes as formatBytes } from "../lib/format";

// nonnegativeLimit parses an optional numeric limit without accidental coercion.
function nonnegativeLimit(value: string): number | null {
  if (!value.trim()) return null;
  const number = Number(value);
  if (!Number.isSafeInteger(number) || number < 0)
    throw new Error("enter a nonnegative whole number");
  return number;
}

// BucketSettings groups the Garage Admin API's bucket configuration operations.
export function BucketSettings({
  bucket,
  session,
  onChanged,
}: {
  /** Current bucket details supplied by the owning workspace. */
  bucket: BucketDetails;
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
  /** Notifies the owner after authoritative resource data changes. */
  onChanged: (bucket: BucketDetails) => void;
}) {
  const canEdit = allows(session, "UpdateBucket");
  const { confirm } = useQuestions();
  const [size, setSize] = useState(
    bucket.quotas?.maxSize == null ? "" : String(bucket.quotas.maxSize),
  );
  const [count, setCount] = useState(
    bucket.quotas?.maxObjects == null ? "" : String(bucket.quotas.maxObjects),
  );
  const [website, setWebsite] = useState(bucket.websiteAccess ?? false);
  const [index, setIndex] = useState(
    bucket.websiteConfig?.indexDocument || "index.html",
  );
  const [errorDocument, setErrorDocument] = useState(
    bucket.websiteConfig?.errorDocument || "",
  );
  const [alias, setAlias] = useState("");
  const [ruleId, setRuleId] = useState("");
  const [rulePrefix, setRulePrefix] = useState("");
  const [expireDays, setExpireDays] = useState("");
  const [abortDays, setAbortDays] = useState("");
  const [rules, setRules] = useState<LifecycleRule[]>(
    bucket.lifecycleRules || [],
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  /** apply saves a bucket mutation and refreshes the authoritative bucket details. */
  async function apply(operation: () => Promise<unknown>): Promise<boolean> {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await operation();
      onChanged(
        await get<BucketDetails>(
          "GetBucketInfo",
          `?id=${encodeURIComponent(bucket.id)}`,
        ),
      );
      setNotice("Saved");
      return true;
    } catch (cause) {
      setError(errorMessage(cause));
      return false;
    } finally {
      setBusy(false);
    }
  }

  /** update submits editable configuration fields for the current bucket. */
  function update(fields: Record<string, unknown>) {
    return post("UpdateBucket", fields, `?id=${encodeURIComponent(bucket.id)}`);
  }

  /** addRule validates and saves a uniquely named lifecycle rule. */
  async function addRule() {
    try {
      const expiration = nonnegativeLimit(expireDays);
      const abort = nonnegativeLimit(abortDays);
      if (!ruleId.trim() || rules.some((rule) => rule.ID === ruleId.trim()))
        throw new Error("rule ID must be unique");
      if (!expiration && !abort)
        throw new Error(
          "specify an expiration or incomplete-upload cleanup period",
        );
      const next: LifecycleRule = { ID: ruleId.trim(), Status: "Enabled" };
      if (rulePrefix.trim()) next.Filter = { Prefix: rulePrefix.trim() };
      if (expiration) next.Expiration = { Days: expiration };
      if (abort)
        next.AbortIncompleteMultipartUpload = { DaysAfterInitiation: abort };
      const updated = [...rules, next];
      if (!(await apply(() => update({ lifecycleRules: updated })))) return;
      setRules(updated);
      setRuleId("");
      setRulePrefix("");
      setExpireDays("");
      setAbortDays("");
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  /** removeRule confirms deletion and persists the remaining lifecycle rules. */
  async function removeRule(index: number) {
    if (!(await confirm("Remove this lifecycle rule?"))) return;
    const updated = rules.filter((_, current) => current !== index);
    if (await apply(() => update({ lifecycleRules: updated })))
      setRules(updated);
  }

  // saveLimits validates optional byte and object quotas before saving the bucket limits.
  async function saveLimits() {
    try {
      const quotas = {
        maxSize: nonnegativeLimit(size),
        maxObjects: nonnegativeLimit(count),
      };
      void apply(() => update({ quotas }));
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  return (
    <section className="detail-section bucket-settings">
      <div className="detail-title">
        <div>
          <h3>Bucket settings</h3>
        </div>
      </div>
      {error && (
        <p className="object-error" role="alert">
          {error}
        </p>
      )}
      {notice && <p className="upload-success">{notice}</p>}
      <div className="bucket-setting-group">
        <h4>Limits</h4>
        <label>
          Maximum storage (bytes; blank = unlimited)
          <input
            inputMode="numeric"
            disabled={!canEdit || busy}
            value={size}
            onChange={(event) => setSize(event.target.value)}
          />
        </label>
        <label>
          Maximum objects
          <input
            inputMode="numeric"
            disabled={!canEdit || busy}
            value={count}
            onChange={(event) => setCount(event.target.value)}
          />
        </label>
        <small>
          Currently {formatBytes(bucket.bytes || 0)} in {bucket.objects || 0}{" "}
          objects.
        </small>
        {canEdit && (
          <button className="secondary" disabled={busy} onClick={saveLimits}>
            <Save size={14} /> Save limits
          </button>
        )}
      </div>
      <div className="bucket-setting-group">
        <h4>Global aliases</h4>
        {(bucket.globalAliases || []).map((value) => (
          <div className="object-toolbar" key={value}>
            <code>{value}</code>
            {allows(session, "RemoveBucketAlias") && (
              <button
                className="icon-button danger"
                title="Remove alias"
                disabled={busy || bucket.globalAliases.length === 1}
                onClick={async () => {
                  if (await confirm(`Remove alias “${value}”?`))
                    void apply(() =>
                      post("RemoveBucketAlias", {
                        bucketId: bucket.id,
                        globalAlias: value,
                      }),
                    );
                }}
              >
                <Trash2 size={15} />
              </button>
            )}
          </div>
        ))}
        {allows(session, "AddBucketAlias") && (
          <div className="object-toolbar">
            <input
              value={alias}
              placeholder="new-alias"
              disabled={busy}
              onChange={(event) => setAlias(event.target.value)}
            />
            <button
              className="secondary"
              disabled={busy || !alias.trim()}
              onClick={() =>
                void apply(async () => {
                  await post("AddBucketAlias", {
                    bucketId: bucket.id,
                    globalAlias: alias.trim(),
                  });
                  setAlias("");
                })
              }
            >
              <Plus size={15} /> Add
            </button>
          </div>
        )}
      </div>
      <div className="bucket-setting-group">
        <h4>Static website</h4>
        <label className="setting-toggle">
          <input
            type="checkbox"
            checked={website}
            disabled={!canEdit || busy}
            onChange={(event) => setWebsite(event.target.checked)}
          />{" "}
          Enable website access
        </label>
        {website && (
          <>
            <label>
              Index document
              <input
                value={index}
                disabled={!canEdit || busy}
                onChange={(event) => setIndex(event.target.value)}
              />
            </label>
            <label>
              Error document (optional)
              <input
                value={errorDocument}
                disabled={!canEdit || busy}
                onChange={(event) => setErrorDocument(event.target.value)}
              />
            </label>
          </>
        )}
        {canEdit && (
          <button
            className="secondary"
            disabled={busy || (website && !index.trim())}
            onClick={() =>
              void apply(() =>
                update({
                  websiteAccess: {
                    enabled: website,
                    ...(website
                      ? {
                          indexDocument: index,
                          errorDocument: errorDocument || null,
                        }
                      : {}),
                  },
                }),
              )
            }
          >
            <Save size={14} /> Save website
          </button>
        )}
      </div>
      <div className="bucket-setting-group">
        <h4>Lifecycle rules</h4>
        {rules.map((rule, index) => (
          <div className="object-toolbar" key={`${rule.ID || "rule"}-${index}`}>
            <span>
              <strong>{rule.ID || `Rule ${index + 1}`}</strong> · {rule.Status}{" "}
              {rule.Expiration?.Days
                ? `· Expire after ${rule.Expiration.Days} days`
                : ""}
            </span>
            {canEdit && (
              <button
                className="icon-button danger"
                title="Remove rule"
                disabled={busy}
                onClick={() => void removeRule(index)}
              >
                <Trash2 size={15} />
              </button>
            )}
          </div>
        ))}
        {canEdit && (
          <>
            <label>
              Rule name
              <input
                value={ruleId}
                disabled={busy}
                onChange={(event) => setRuleId(event.target.value)}
                placeholder="old-objects"
              />
            </label>
            <label>
              Object prefix (optional)
              <input
                value={rulePrefix}
                disabled={busy}
                onChange={(event) => setRulePrefix(event.target.value)}
                placeholder="backups/"
              />
            </label>
            <label>
              Expire objects after days
              <input
                inputMode="numeric"
                value={expireDays}
                disabled={busy}
                onChange={(event) => setExpireDays(event.target.value)}
              />
            </label>
            <label>
              Abort incomplete uploads after days
              <input
                inputMode="numeric"
                value={abortDays}
                disabled={busy}
                onChange={(event) => setAbortDays(event.target.value)}
              />
            </label>
            <button
              className="secondary"
              disabled={busy || !ruleId}
              onClick={() => void addRule()}
            >
              <Plus size={14} /> Add rule
            </button>
          </>
        )}
      </div>
    </section>
  );
}
