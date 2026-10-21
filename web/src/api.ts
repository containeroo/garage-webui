import type { AdminOperation } from "./admin-operation";
import { CredentialKind } from "./domain.ts";
/** The current Garage credential metadata, excluding credential secrets. */
export type Session = {
  /** Credential kind; empty for an unauthenticated session. */
  mode: CredentialKind | "";
  /** Display label returned for the authenticated credential. */
  user: string;
  /** Garage operations permitted by the admin token. */
  scope: string[] | null;
  /** Whether the deployment exposes a browser-accessible S3 endpoint. */
  uploadsEnabled: boolean;
  /** Whether optional public signed transfers are configured. */
  directTransfersEnabled?: boolean;
};
/** allows checks the credential's Garage admin scopes for a requested operation. */
export const allows = (session: Session, operation: AdminOperation) =>
  session.mode === CredentialKind.Admin &&
  !!(session.scope?.includes("*") || session.scope?.includes(operation));
/** A Garage bucket summary returned by the list operation. */
export type Bucket = {
  /** Stable Garage identifier for this resource. */
  id: string;
  /** Globally visible names associated with the bucket. */
  globalAliases: string[];
  /** Bucket creation timestamp when provided by Garage. */
  created?: string;
};
/** A Garage S3 key summary without its secret. */
export type AccessKey = {
  /** Stable Garage identifier for this resource. */
  id: string;
  /** Human-readable resource label. */
  name: string;
  /** Whether Garage considers this credential expired. */
  expired: boolean;
};
/** The cluster's current connectivity, storage, and partition health. */
export type Health = {
  /** Current health or transfer state. */
  status: string;
  /** Number of nodes currently connected to Garage. */
  connectedNodes: number;
  /** Total number of nodes known to the cluster. */
  knownNodes: number;
  /** Number of online storage nodes. */
  storageNodesUp: number;
  /** Total number of assigned storage nodes. */
  storageNodes: number;
  /** Partitions with every expected replica healthy. */
  partitionsAllOk: number;
  /** Partitions with enough replicas to satisfy quorum. */
  partitionsQuorum: number;
  /** Total number of cluster partitions. */
  partitions: number;
};
/** An access key's permissions on one bucket. */
export type Grant = {
  /** Garage S3 access-key identifier, excluding its secret. */
  accessKeyId: string;
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
/** An S3 lifecycle rule using the Garage API's field casing. */
export type LifecycleRule = {
  /** Unique lifecycle-rule identifier within the bucket. */
  ID?: string;
  /** Whether this lifecycle rule is enabled or disabled. */
  Status: "Enabled" | "Disabled";
  /** Optional restriction to object keys with a matching prefix. */
  Filter?: {
    /** Object-key prefix to which this rule applies. */
    Prefix?: string;
  };
  /** Automatic object-expiration policy. */
  Expiration?: {
    /** Number of days before objects expire. */
    Days: number;
  };
  /** Cleanup policy for unfinished multipart uploads. */
  AbortIncompleteMultipartUpload?: {
    /** Days after initiation before an unfinished upload is removed. */
    DaysAfterInitiation: number;
  };
};
/** The selected bucket's configuration, grants, and usage counters. */
export type BucketDetails = {
  /** Stable Garage identifier for this resource. */
  id: string;
  /** Access-key summaries or grants available to the current view. */
  keys: Grant[];
  /** Globally visible names associated with the bucket. */
  globalAliases: string[];
  /** Optional limits on stored bytes and object count. */
  quotas?: {
    /** Maximum stored bytes; null removes the limit. */
    maxSize: number | null;
    /** Maximum object count; null removes the limit. */
    maxObjects: number | null;
  };
  /** Whether public static website access is enabled. */
  websiteAccess?: boolean;
  /** Static website index and optional error documents. */
  websiteConfig?: {
    /** Default document served for website directory requests. */
    indexDocument: string;
    /** Optional document served for website errors. */
    errorDocument?: string | null;
  };
  /** Lifecycle policies currently configured on the bucket. */
  lifecycleRules?: LifecycleRule[];
  /** Total number of stored objects. */
  objects?: number;
  /** Total stored object bytes. */
  bytes?: number;
};
/** A supported top-level console destination. */
export type WorkspacePage =
  "overview" | "cluster" | "buckets" | "jobs" | "keys" | "tokens" | "profile";

// isRecord narrows untrusted JSON to an object with inspectable fields.
function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

// fieldProblems extracts readable field validation errors from an API problem.
function fieldProblems(details: unknown): string[] {
  if (!isRecord(details)) return [];
  return Object.entries(details)
    .filter(
      (entry): entry is [string, string] =>
        typeof entry[1] === "string" && entry[1].trim() !== "",
    )
    .map(([field, detail]) => `${field.replaceAll("_", " ")}: ${detail}`);
}

// errorMessage turns caught exceptions into text suitable for display.
export function errorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  const normalized = message.replace(
    /^(\s*)([A-Z]{2,})(?=\W|$)/,
    (_, whitespace: string, acronym: string) =>
      whitespace + acronym.toLowerCase(),
  );

  return normalized.replace(
    /^(\s*)(\p{Lu})/u,
    (_, whitespace: string, first: string) => whitespace + first.toLowerCase(),
  );
}

// responseProblem accepts our API problems, Garage errors, and plain-text errors.
export async function responseProblem(response: Response): Promise<Error> {
  const fallback = `request failed (HTTP ${response.status}).`;
  const text = (await response.text()).trim();
  if (!text) return new Error(fallback);
  try {
    const payload: unknown = JSON.parse(text);
    if (isRecord(payload)) {
      const problem = payload;
      const message = [problem.error, problem.message].find(
        (value): value is string =>
          typeof value === "string" && value.trim() !== "",
      );
      const fields = fieldProblems(problem.problems);
      return new Error([message?.trim() || fallback, ...fields].join("\n"));
    }
    return new Error(fallback);
  } catch {
    // Proxies may return HTML; avoid displaying markup as an error message.
    if (
      response.headers.get("content-type")?.includes("text/html") ||
      /^\s*</.test(text)
    ) {
      return new Error(fallback);
    }
    return new Error(text);
  }
}

// request calls the console API and preserves caller headers and cancellation.
export async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  let response: Response;
  const headers = new Headers(options?.headers);
  if (!headers.has("Content-Type"))
    headers.set("Content-Type", "application/json");
  try {
    response = await fetch(new URL(path, document.baseURI), {
      ...options,
      headers,
    });
  } catch (cause) {
    if (options?.signal?.aborted) throw cause;
    throw new Error(
      "could not connect to the server. Check your connection and try again.",
      { cause },
    );
  }
  if (!response.ok) {
    throw await responseProblem(response);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
/** get retrieves an authorized Garage operation with optional query and cancellation settings. */
export const get = <T>(op: AdminOperation, query = "", options?: RequestInit) =>
  request<T>(`api/garage/${op}${query}`, options);
/** post submits a JSON mutation to an authorized Garage operation. */
export const post = <T>(op: AdminOperation, data: unknown = {}, query = "") =>
  request<T>(`api/garage/${op}${query}`, {
    method: "POST",
    body: JSON.stringify(data),
  });
/** short abbreviates long identifiers for compact display without changing their underlying value. */
export const short = (id: string) =>
  id.length > 18 ? `${id.slice(0, 14)}…` : id;
