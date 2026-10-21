import { useEffect, useState } from "react";
import { cancelDeletion, listDeletions, type DeletionJob } from "../jobs";
import { errorMessage } from "../api";

/** BackgroundJobs keeps deletion progress visible across tabs and browser reopenings. */
export function BackgroundJobs({
  onCompleted,
}: {
  /** Refreshes the current object listing when a job completes. */
  onCompleted?: () => void;
}) {
  const [jobs, setJobs] = useState<DeletionJob[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const known = new Map<string, string>();
    /** update merges fresh server state and detects completed tasks without guessing progress. */
    function update(next: DeletionJob[]) {
      if (!active || !Array.isArray(next)) return;
      for (const job of next) {
        const previous = known.get(job.id);
        if (previous && previous !== job.status && job.status === "completed")
          onCompleted?.();
        known.set(job.id, job.status);
      }
      setJobs(next);
    }
    void listDeletions()
      .then(update)
      .catch((cause) => {
        if (active) setError(errorMessage(cause));
      });
    // Native EventSource automatically reconnects when the tab regains connectivity.
    if (typeof EventSource !== "undefined") {
      const stream = new EventSource(
        new URL("api/jobs/events", document.baseURI),
      );
      stream.addEventListener("jobs", (event: Event) => {
        try {
          update(
            JSON.parse((event as MessageEvent<string>).data) as DeletionJob[],
          );
        } catch {
          if (active) setError("invalid background job update");
        }
      });
      return () => {
        active = false;
        stream.close();
      };
    }
    // Tests/older browsers without SSE still load the persisted snapshot.
    return () => {
      active = false;
    };
  }, [onCompleted]);
  const recent = jobs.slice(0, 10);
  if (!recent.length && !error) return null;
  return (
    <section
      className="panel background-jobs"
      aria-label="Background operations"
    >
      <h2>Background operations</h2>
      {error && <p role="alert">{error}</p>}
      {recent.map((job) => (
        <div
          key={job.id}
          className="explorer-message"
          aria-label={`Deletion job ${job.id}`}
        >
          <strong>
            {job.bucket}/{job.target || "(selection)"}
          </strong>
          : {job.status}
          {job.cancelRequested ? " (cancelling…)" : ""} · {job.deleted} objects
          deleted
          {job.error && <p role="alert">{job.error}</p>}
          {(job.status === "pending" || job.status === "running") &&
            !job.cancelRequested && (
              <button
                className="text-button"
                onClick={() =>
                  void cancelDeletion(job.id)
                    .then(() =>
                      setJobs((current) =>
                        current.map((item) =>
                          item.id === job.id
                            ? { ...item, cancelRequested: true }
                            : item,
                        ),
                      ),
                    )
                    .catch((cause) => setError(errorMessage(cause)))
                }
              >
                Cancel
              </button>
            )}
        </div>
      ))}
    </section>
  );
}
