import { useQuestions } from "./ui/questions";
import { errorMessage } from "../api";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  ChevronRight,
  Home,
  Search,
  UploadCloud,
  X,
  CheckSquare,
  Copy,
  Download,
  Eye,
  File,
  Folder,
  LoaderCircle,
  MoveRight,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { Empty } from "./storage";
import {
  copyObject,
  deleteObject,
  listObjects,
  signOperation,
  type ObjectItem,
  type ObjectPage,
} from "../s3";
import { formatBinaryBytes as formatBytes } from "../lib/format";

const emptyPage: ObjectPage = { items: [], folders: [], nextToken: "" };

// ObjectPreview renders only preview content belonging to the currently selected object.
function ObjectPreview({
  preview,
  item,
}: {
  /** Authorized preview metadata, when a preview has been requested. */
  preview?: Preview;
  /** Object whose details and preview are currently selected. */
  item: ObjectItem;
}) {
  if (preview?.key !== item.key) return <File size={48} strokeWidth={1.3} />;
  switch (preview.kind) {
    case "image":
      return <img src={preview.url} alt={item.key} />;
    case "video":
      return <video src={preview.url} controls />;
    case "audio":
      return <audio src={preview.url} controls />;
    case "text":
      return <pre>{preview.content}</pre>;
    default:
      return <File size={48} strokeWidth={1.3} />;
  }
}
/** An ordering available for objects on the current page. */
type SortMode = "name" | "size" | "modified";

/** The active object's signed preview and optional bounded text content. */
type Preview = {
  /** Short-lived authorized URL for this browser operation. */
  url: string;
  /** Complete object key, including any folder prefix. */
  key: string;
  /** Safe inline preview format, or an external-file fallback. */
  kind: "image" | "video" | "audio" | "text" | "external";
  /** Fetched text content for a bounded inline preview. */
  content?: string;
};

// previewKind maps a bounded set of safe formats to inline displays.
function previewKind(key: string): Preview["kind"] {
  const extension = key.split(".").at(-1)?.toLowerCase();
  if (
    ["jpg", "jpeg", "png", "gif", "webp", "svg", "avif"].includes(
      extension || "",
    )
  )
    return "image";
  if (["mp4", "webm"].includes(extension || "")) return "video";
  if (["mp3", "wav", "ogg"].includes(extension || "")) return "audio";
  if (
    ["txt", "md", "json", "csv", "log", "yaml", "yml"].includes(extension || "")
  )
    return "text";
  return "external";
}

// compareItems sorts only the displayed page, not unvisited S3 pages.
function compareItems(a: ObjectItem, b: ObjectItem, sort: SortMode): number {
  if (sort === "size") return a.size - b.size || a.key.localeCompare(b.key);
  if (sort === "modified")
    return a.modified.localeCompare(b.modified) || a.key.localeCompare(b.key);
  return a.key.localeCompare(b.key);
}

// BucketObjects displays folders and one page of objects in a selected bucket.
export function BucketObjects({
  bucketId,
  canDelete,
  enabled,
  refreshVersion,
  onFolderChange,
  onUpload,
}: {
  /** Identifier or accessible alias of the bucket being managed. */
  bucketId: string;
  /** Whether mutation controls should be offered; Garage still enforces actual permissions. */
  canDelete: boolean;
  /** Whether direct S3 operations are configured for this deployment. */
  enabled: boolean;
  /** Owner-controlled counter that invalidates the displayed listing. */
  refreshVersion: number;
  /** Reports the newly selected folder prefix to the owner. */
  onFolderChange?: (prefix: string) => void;
  /** Opens the file-upload dialog for the selected folder. */
  onUpload?: () => void;
}) {
  const { confirm, prompt } = useQuestions();
  const [page, setPage] = useState<ObjectPage>(emptyPage);
  const [prefix, setPrefix] = useState("");
  const [query, setQuery] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("name");
  const [descending, setDescending] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const [token, setToken] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [activeItem, setActiveItem] = useState<ObjectItem>();
  const [preview, setPreview] = useState<Preview>();
  const [version, setVersion] = useState(0);
  const previewRequest = useRef<AbortController | null>(null);
  useEffect(() => () => previewRequest.current?.abort(), []);

  // clearPreview cancels a preview belonging to a deselected object or folder.
  function clearPreview() {
    previewRequest.current?.abort();
    setPreview(undefined);
  }

  const listPrefix = prefix + appliedQuery;
  const ordered = useMemo(() => {
    const items = [...page.items].sort((a, b) => compareItems(a, b, sort));
    return descending ? items.reverse() : items;
  }, [page.items, sort, descending]);

  // load fetches the selected folder and page directly from S3.
  const load = useCallback(
    async (continuation: string, signal?: AbortSignal) => {
      if (!enabled) return;
      setLoading(true);
      setError("");
      try {
        const result = await listObjects(
          bucketId,
          listPrefix,
          continuation,
          signal,
        );
        if (!signal?.aborted) setPage(result);
      } catch (cause) {
        if (!signal?.aborted) setError(errorMessage(cause));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [bucketId, enabled, listPrefix],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(token, controller.signal);
    return () => controller.abort();
  }, [load, token, refreshVersion, version]);

  /** navigate changes folders and clears the previous page's selection and preview. */
  function navigate(folder: string) {
    setActiveItem(undefined);
    clearPreview();
    setPrefix(folder);
    setAppliedQuery("");
    setQuery("");
    resetPage();
    onFolderChange?.(folder);
  }

  /** resetPage returns pagination and selection to the first page of the current listing. */
  function resetPage() {
    setToken("");
    setHistory([]);
    setSelected([]);
    setPage(emptyPage);
  }
  /** refresh retrieves the current view's authoritative data before updating its state. */
  function refresh() {
    setVersion((old) => old + 1);
  }
  /** displayedName removes the current folder prefix from an object's visible name. */
  function displayedName(key: string) {
    return key.startsWith(prefix) ? key.slice(prefix.length) : key;
  }

  /** download obtains an authorized URL and starts the browser download. */
  async function download(key: string) {
    setError("");
    try {
      const signed = await signOperation(bucketId, "download", { key });
      window.location.assign(signed.url);
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  /** openPreview signs an object preview and bounds inline text retrieval. */
  async function openPreview(item: ObjectItem) {
    clearPreview();
    const controller = new AbortController();
    previewRequest.current = controller;
    setError("");
    try {
      const signed = await signOperation(
        bucketId,
        "preview",
        { key: item.key },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const kind = previewKind(item.key);
      if (kind !== "text") {
        setPreview({ key: item.key, url: signed.url, kind });
        return;
      }
      if (item.size > 1024 * 1024)
        throw new Error(
          "text preview is limited to 1 MiB. Download the file instead.",
        );
      const response = await fetch(signed.url, {
        credentials: "omit",
        signal: controller.signal,
      });
      if (!response.ok)
        throw new Error(`preview failed (HTTP ${response.status})`);
      const content = await response.text();
      if (controller.signal.aborted) return;
      setPreview({
        key: item.key,
        url: signed.url,
        kind,
        content,
      });
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorMessage(cause));
    }
  }

  /** remove confirms deletion and reports failures before refreshing the affected view. */
  async function remove(keys: string[]) {
    if (
      !keys.length ||
      !(await confirm(
        `Delete ${keys.length} selected object(s)? This cannot be undone.`,
      ))
    )
      return;
    setBusy(true);
    setError("");
    const failures: string[] = [];
    try {
      // Sequential deletes avoid issuing a burst of privileged presigned URLs.
      for (const key of keys) {
        try {
          await deleteObject(bucketId, key);
        } catch (cause) {
          failures.push(`${key}: ${errorMessage(cause)}`);
        }
      }
      setSelected([]);
      setActiveItem(undefined);
      clearPreview();
      resetPage();
      refresh();
      if (failures.length)
        setError(
          `${failures.length} deletions failed: ${failures.slice(0, 3).join("; ")}`,
        );
    } finally {
      setBusy(false);
    }
  }

  /** relocate copies or moves an object after validating the requested destination. */
  async function relocate(source: string, size: number, move: boolean) {
    const target = await prompt(
      `${move ? "Move / rename" : "Copy"} “${source}” to object key (same bucket):`,
      source,
    );
    if (target === null || target === source || !target.trim()) return;
    if (
      page.items.some((item) => item.key === target) &&
      !(await confirm(`Overwrite “${target}”?`))
    )
      return;
    setBusy(true);
    setError("");
    try {
      await copyObject(bucketId, source, target, size);
      if (move) {
        await deleteObject(bucketId, source);
        setActiveItem(undefined);
        clearPreview();
      }
      resetPage();
      refresh();
    } catch (cause) {
      setError(`copy${move ? " / move" : ""} failed: ${errorMessage(cause)}`);
    } finally {
      setBusy(false);
    }
  }

  /** toggle adds or removes one object key from the current page selection. */
  function toggle(key: string) {
    setSelected((current) =>
      current.includes(key)
        ? current.filter((candidate) => candidate !== key)
        : [...current, key],
    );
  }

  const segments = prefix.split("/").filter(Boolean);
  return (
    <section className="bucket-objects" aria-label="Object browser">
      {!enabled ? (
        <Empty
          text="S3 endpoint not configured"
          detail="Set the S3 public URL to manage objects."
        />
      ) : (
        <div className="explorer-layout">
          <div className="explorer-main">
            <div className="explorer-toolbar">
              <button
                className="icon-button"
                aria-label="Parent folder"
                title="Parent folder"
                disabled={!prefix}
                onClick={() => navigate(prefix.replace(/[^/]+\/$/, ""))}
              >
                <ArrowLeft size={17} />
              </button>
              <nav className="explorer-path" aria-label="Folder path">
                <button
                  title="Bucket root"
                  aria-label="Bucket root"
                  onClick={() => navigate("")}
                >
                  <Home size={16} />
                </button>
                {segments.map((part, index) => (
                  <span key={index}>
                    <ChevronRight size={14} />
                    <button
                      onClick={() =>
                        navigate(segments.slice(0, index + 1).join("/") + "/")
                      }
                    >
                      {part}
                    </button>
                  </span>
                ))}
              </nav>
              <form
                className="explorer-search"
                onSubmit={(event) => {
                  event.preventDefault();
                  setAppliedQuery(query.trim());
                  resetPage();
                  setActiveItem(undefined);
                  clearPreview();
                  refresh();
                }}
              >
                <Search size={16} />
                <input
                  aria-label="Filter by object key prefix"
                  placeholder="Filter by name prefix"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <button type="submit" className="text-button">
                  Find
                </button>
              </form>
              <button
                className="icon-button"
                title="Refresh objects"
                aria-label="Refresh objects"
                disabled={loading}
                onClick={refresh}
              >
                <RefreshCw size={16} />
              </button>
              {canDelete && onUpload && (
                <button
                  className="icon-button"
                  title="Upload object"
                  aria-label="Upload object"
                  onClick={onUpload}
                >
                  <UploadCloud size={17} />
                </button>
              )}
            </div>
            <div className="explorer-options">
              <label>
                Sort current page{" "}
                <select
                  value={sort}
                  onChange={(event) => setSort(event.target.value as SortMode)}
                >
                  <option value="name">Name</option>
                  <option value="size">Size</option>
                  <option value="modified">Modified</option>
                </select>
              </label>
              <button
                className="text-button"
                onClick={() => setDescending((value) => !value)}
              >
                {descending ? "Descending" : "Ascending"}
              </button>
              {canDelete && page.items.length > 0 && (
                <>
                  <button
                    className="text-button"
                    onClick={() =>
                      setSelected(
                        selected.length === page.items.length
                          ? []
                          : page.items.map((item) => item.key),
                      )
                    }
                  >
                    <CheckSquare size={14} />
                    {selected.length === page.items.length
                      ? "Deselect all"
                      : "Select page"}
                  </button>
                  {selected.length > 0 && (
                    <button
                      className="text-button danger"
                      disabled={busy}
                      onClick={() => void remove(selected)}
                    >
                      <Trash2 size={14} />
                      Delete {selected.length}
                    </button>
                  )}
                </>
              )}
            </div>
            {error && (
              <p className="object-error explorer-message" role="alert">
                {error}
              </p>
            )}
            <div className="explorer-table" aria-label="Objects">
              <div className="explorer-heading">
                <span aria-hidden="true" />
                <span>Name</span>
                <span>Size</span>
                <span>Last modified</span>
                <span />
              </div>
              {loading && (
                <p className="object-loading explorer-message">
                  <LoaderCircle className="spin" size={16} /> Loading objects…
                </p>
              )}
              {!loading &&
                !error &&
                page.folders.length === 0 &&
                page.items.length === 0 && (
                  <Empty
                    text="Nothing here"
                    detail="Upload files or change the prefix filter."
                  />
                )}
              {page.folders.map((folder) => (
                <button
                  key={folder}
                  className="explorer-row explorer-folder"
                  onClick={() => navigate(folder)}
                >
                  <span />
                  <span className="explorer-name">
                    <Folder size={17} />
                    <strong>{folder.slice(prefix.length)}</strong>
                  </span>
                  <span>—</span>
                  <span>—</span>
                  <ChevronRight size={16} />
                </button>
              ))}
              {ordered.map((item) => (
                <div
                  className={`explorer-row ${activeItem?.key === item.key ? "active" : ""}`}
                  key={item.key}
                >
                  <span>
                    {canDelete && (
                      <input
                        type="checkbox"
                        aria-label={`Select ${item.key}`}
                        checked={selected.includes(item.key)}
                        disabled={busy}
                        onChange={() => toggle(item.key)}
                      />
                    )}
                  </span>
                  <button
                    className="explorer-name"
                    onClick={() => {
                      setActiveItem(item);
                      clearPreview();
                    }}
                  >
                    <File size={17} />
                    <strong title={item.key}>{displayedName(item.key)}</strong>
                  </button>
                  <span>{formatBytes(item.size)}</span>
                  <span>
                    {item.modified
                      ? new Date(item.modified).toLocaleString()
                      : "—"}
                  </span>
                  <span className="explorer-row-actions">
                    <button
                      className="icon-button"
                      title="Preview"
                      disabled={busy}
                      onClick={() => {
                        setActiveItem(item);
                        void openPreview(item);
                      }}
                    >
                      <Eye size={16} />
                    </button>
                    <button
                      className="icon-button"
                      title="Download"
                      disabled={busy}
                      onClick={() => void download(item.key)}
                    >
                      <Download size={16} />
                    </button>
                  </span>
                </div>
              ))}
            </div>
            {(history.length > 0 || page.nextToken) && (
              <div className="object-pagination">
                <button
                  className="secondary"
                  disabled={loading || history.length === 0}
                  onClick={() => {
                    setToken(history.at(-1) || "");
                    setHistory((tokens) => tokens.slice(0, -1));
                    setSelected([]);
                    setActiveItem(undefined);
                    clearPreview();
                  }}
                >
                  Previous
                </button>
                <span>Page {history.length + 1}</span>
                <button
                  className="secondary"
                  disabled={loading || !page.nextToken}
                  onClick={() => {
                    setHistory((tokens) => [...tokens, token]);
                    setToken(page.nextToken);
                    setSelected([]);
                    setActiveItem(undefined);
                    clearPreview();
                  }}
                >
                  Next
                </button>
              </div>
            )}
          </div>
          <aside className="explorer-details" aria-label="Object details">
            <div className="explorer-details-head">
              <strong>
                {activeItem ? displayedName(activeItem.key) : "Details"}
              </strong>
              {activeItem && (
                <button
                  className="icon-button"
                  aria-label="Close object details"
                  onClick={() => {
                    setActiveItem(undefined);
                    clearPreview();
                  }}
                >
                  <X size={16} />
                </button>
              )}
            </div>
            {activeItem ? (
              <>
                <div className="explorer-preview">
                  <ObjectPreview preview={preview} item={activeItem} />
                </div>
                <dl className="explorer-metadata">
                  <div>
                    <dt>Size</dt>
                    <dd>
                      {formatBytes(activeItem.size)} (
                      {activeItem.size.toLocaleString()} bytes)
                    </dd>
                  </div>
                  <div>
                    <dt>Modified</dt>
                    <dd>
                      {activeItem.modified
                        ? new Date(activeItem.modified).toLocaleString()
                        : "—"}
                    </dd>
                  </div>
                  <div>
                    <dt>Path</dt>
                    <dd>{activeItem.key}</dd>
                  </div>
                </dl>
                <div className="explorer-detail-actions">
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() => void download(activeItem.key)}
                  >
                    <Download size={15} />
                    Download
                  </button>
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() => void openPreview(activeItem)}
                  >
                    <Eye size={15} />
                    Preview
                  </button>
                  {preview?.key === activeItem.key &&
                    preview.kind === "external" && (
                      <a
                        className="secondary"
                        href={preview.url}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        Open in new tab
                      </a>
                    )}
                  {canDelete && (
                    <>
                      <button
                        className="secondary"
                        disabled={busy}
                        onClick={() =>
                          void relocate(activeItem.key, activeItem.size, false)
                        }
                      >
                        <Copy size={15} />
                        Copy
                      </button>
                      <button
                        className="secondary"
                        disabled={busy}
                        onClick={() =>
                          void relocate(activeItem.key, activeItem.size, true)
                        }
                      >
                        <MoveRight size={15} />
                        Move or rename
                      </button>
                      <button
                        className="secondary danger explorer-delete"
                        disabled={busy}
                        onClick={() => void remove([activeItem.key])}
                      >
                        <Trash2 size={15} />
                        Delete
                      </button>
                    </>
                  )}
                </div>
              </>
            ) : (
              <div className="explorer-placeholder">
                <File size={28} />
                <span>Select a file to see its details.</span>
              </div>
            )}
          </aside>
        </div>
      )}
    </section>
  );
}
