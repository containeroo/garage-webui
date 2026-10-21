import { request } from "./api";

/** DeletionJob describes persisted progress available to the initiating Garage credential. */
export type DeletionJob = {
  /** Unique stable job identifier. */
  id: string;
  /** Garage bucket selected for deletion. */
  bucket: string;
  /** Display label for the first selected file or folder. */
  target?: string;
  /** Current lifecycle status. */
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  /** Successfully processed objects. */
  deleted: number;
  /** Safe failure description, if any. */
  error?: string;
  /** Whether cancellation has been requested. */
  cancelRequested: boolean;
  /** Job acceptance timestamp. */
  createdAt: string;
  /** Latest persisted update timestamp. */
  updatedAt: string;
};

/** startDeletion queues selected exact object keys and recursive folder prefixes. */
export function startDeletion(
  bucket: string,
  files: string[],
  folders: string[],
): Promise<DeletionJob> {
  return request<DeletionJob>("api/jobs/deletions", {
    method: "POST",
    body: JSON.stringify({ bucket, files, folders }),
  });
}

/** listDeletions loads the last 100 jobs belonging to the signed-in credential. */
export function listDeletions(): Promise<DeletionJob[]> {
  return request<DeletionJob[]>("api/jobs");
}

/** cancelDeletion asks a worker to stop safely after its current operation. */
export function cancelDeletion(id: string): Promise<void> {
  return request<void>(`api/jobs/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
    body: "{}",
  });
}
