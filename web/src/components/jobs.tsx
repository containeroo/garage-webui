import { useCallback, useEffect, useState } from "react";
import { cancelDeletion, listDeletions, type DeletionJob } from "../jobs";
import { errorMessage } from "../api";

/** BackgroundJobs shows persisted job state when the user asks to refresh it. */
export function BackgroundJobs() {
  const [jobs, setJobs] = useState<DeletionJob[]>([]);
  const [error, setError] = useState("");
  const [refreshing, setRefreshing] = useState(false);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    setError("");
    try {
      const next = await listDeletions();
      if (!Array.isArray(next)) {
        return;
      }
      setJobs(next);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const recent = jobs.slice(0, 10);
  return (
    <section
      className="panel background-jobs"
      aria-label="Background operations"
    >
      <h2>
        Background operations{" "}
        <button
          className="text-button"
          onClick={() => void refresh()}
          disabled={refreshing}
        >
          {refreshing ? "Refreshing…" : "Refresh status"}
        </button>
      </h2>
      {error && <p role="alert">{error}</p>}
      {!recent.length && !error && <p>No recent background operations.</p>}
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
                    .then(refresh)
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
