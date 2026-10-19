import { errorMessage } from "../api";
import { useEffect, useRef, useState, type DragEvent } from "react";
import { CircleAlert, LoaderCircle, UploadCloud, X } from "lucide-react";
import { uploadObject, formatBytes, type TransferProgress } from "../s3";
import { DialogPanel } from "./ui/dialog";
import { Button } from "./ui/button";

type QueuedFile = {
  file: File;
  key: string;
  status: "pending" | "uploading" | "done" | "failed";
  error?: string;
};

// formatTime shows an approximate ETA while a file is transferring.
function formatTime(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds)) return "Estimating…";
  if (seconds < 60) return `${Math.ceil(seconds)}s`;
  if (seconds < 3600)
    return `${Math.floor(seconds / 60)}m ${Math.ceil(seconds % 60)}s`;
  return `${Math.floor(seconds / 3600)}h ${Math.ceil((seconds % 3600) / 60)}m`;
}

// validPrefix keeps user-selected upload folders distinct from file names.
function normalizedPrefix(value: string): string {
  const prefix = value.trim().replace(/\/+$/, "");
  if (!prefix) return "";
  if (
    prefix.startsWith("/") ||
    prefix.split("/").some((part) => !part || part === "." || part === "..")
  ) {
    throw new Error(
      "enter a folder without leading slashes or traversal components",
    );
  }
  return prefix + "/";
}

// UploadDialog queues files and uploads them directly to Garage, one at a time.
export function UploadDialog({
  bucketId,
  bucketName,
  initialPrefix = "",
  onClose,
  onUploaded,
}: {
  bucketId: string;
  bucketName: string;
  initialPrefix?: string;
  onClose: () => void;
  onUploaded: () => void;
}) {
  const [queue, setQueue] = useState<QueuedFile[]>([]);
  const [prefix, setPrefix] = useState(initialPrefix.replace(/\/$/, ""));
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [progress, setProgress] = useState<TransferProgress>({
    loaded: 0,
    total: 0,
    speed: 0,
    eta: null,
  });
  const [error, setError] = useState("");
  const controller = useRef<AbortController | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

  function addFiles(files: FileList | File[]) {
    const selected = Array.from(files);
    setQueue((current) => [
      ...current,
      ...selected.map((file) => ({
        file,
        key: file.name,
        status: "pending" as const,
      })),
    ]);
    setError("");
  }

  function dropped(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    addFiles(event.dataTransfer.files);
  }

  function cancel() {
    controller.current?.abort();
  }

  async function start() {
    if (
      busy ||
      !queue.some(
        (entry) => entry.status === "pending" || entry.status === "failed",
      )
    )
      return;
    let folder: string;
    try {
      folder = normalizedPrefix(prefix);
    } catch (cause) {
      setError(errorMessage(cause));
      return;
    }
    const current = new AbortController();
    controller.current = current;
    setBusy(true);
    setError("");
    const pending = queue.map((entry) => ({ ...entry }));
    let anyUploaded = false;
    for (let i = 0; i < pending.length; i++) {
      const entry = pending[i];
      if (current.signal.aborted) break;
      if (entry.status === "done") continue;
      pending[i] = { ...entry, status: "uploading", error: undefined };
      setQueue([...pending]);
      const started = performance.now();
      setProgress({ loaded: 0, total: entry.file.size, speed: 0, eta: null });
      try {
        await uploadObject(
          bucketId,
          folder + entry.key,
          entry.file,
          current.signal,
          (loaded) => {
            const elapsed = Math.max(
              (performance.now() - started) / 1000,
              0.01,
            );
            const speed = loaded / elapsed;
            setProgress({
              loaded,
              total: entry.file.size,
              speed,
              eta: speed > 0 ? (entry.file.size - loaded) / speed : null,
            });
          },
        );
        pending[i] = { ...entry, status: "done" };
        anyUploaded = true;
        onUploaded();
      } catch (cause) {
        pending[i] = {
          ...entry,
          status: "failed",
          error: errorMessage(cause),
        };
        setQueue([...pending]);
        if (current.signal.aborted) break;
      }
      setQueue([...pending]);
    }
    if (current.signal.aborted && !anyUploaded) setError("uploads cancelled");
    controller.current = null;
    setBusy(false);
  }

  const completed = queue.filter((entry) => entry.status === "done").length;
  const current = queue.find((entry) => entry.status === "uploading");
  const percentage = progress.total
    ? Math.round((progress.loaded / progress.total) * 100)
    : 0;
  return (
    <DialogPanel
      title={`Upload to ${bucketName}`}
      onClose={() => {
        cancel();
        onClose();
      }}
    >
      <div className="upload-dialog">
        <label>
          Destination folder
          <input
            value={prefix}
            disabled={busy}
            placeholder="Optional folder/subfolder"
            onChange={(event) => setPrefix(event.target.value)}
          />
        </label>
        <div
          className={`upload-dropzone ${dragging ? "is-dragging" : ""}`}
          onDragOver={(event) => {
            event.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={dropped}
        >
          <UploadCloud size={24} />
          <span>Drop files here or select multiple files</span>
          <input
            type="file"
            multiple
            disabled={busy}
            onChange={(event) => {
              if (event.currentTarget.files)
                addFiles(event.currentTarget.files);
              event.currentTarget.value = "";
            }}
          />
        </div>
        {queue.length > 0 && (
          <div className="upload-queue">
            <div className="upload-queue-head">
              <strong>
                {completed} / {queue.length} completed
              </strong>
              {!busy && (
                <button
                  type="button"
                  className="text-button"
                  onClick={() => setQueue([])}
                >
                  Clear
                </button>
              )}
            </div>
            {queue.map((entry, index) => (
              <div className="upload-queue-row" key={`${entry.key}-${index}`}>
                <div>
                  <strong>{entry.key}</strong>
                  <small>
                    {formatBytes(entry.file.size)} · {entry.status}
                  </small>
                  {entry.error && (
                    <small className="object-error">{entry.error}</small>
                  )}
                </div>
                {!busy && entry.status !== "done" && (
                  <button
                    type="button"
                    className="icon-button"
                    title="Remove file"
                    onClick={() =>
                      setQueue((items) => items.filter((_, i) => i !== index))
                    }
                  >
                    <X size={15} />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
        {current && (
          <div className="upload-progress" role="status">
            <div className="upload-progress-header">
              <span className="upload-status-label">
                <LoaderCircle size={16} className="spin" /> {current.key}
              </span>
              <strong>{percentage}%</strong>
            </div>
            <progress max={100} value={percentage} />
            <div className="upload-progress-details">
              <span>
                {formatBytes(progress.loaded)} / {formatBytes(progress.total)}
              </span>
              <span>
                {formatBytes(progress.speed)}/s · {formatTime(progress.eta)}{" "}
                remaining
              </span>
            </div>
          </div>
        )}
        {error && (
          <p className="upload-error">
            <CircleAlert size={16} />
            {error}
          </p>
        )}
        <div className="modal-actions">
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              cancel();
              if (!busy) onClose();
            }}
          >
            {busy ? "Cancel uploads" : "Close"}
          </Button>
          <Button
            type="button"
            disabled={busy || !queue.some((entry) => entry.status !== "done")}
            onClick={() => void start()}
          >
            <UploadCloud size={16} /> Upload{" "}
            {queue.length > 1 ? "files" : "file"}
          </Button>
        </div>
      </div>
    </DialogPanel>
  );
}
