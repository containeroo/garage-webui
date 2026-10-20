export type Session = {
  mode: "admin" | "s3" | "";
  user: string;
  scope: string[];
  uploadsEnabled: boolean;
};
export const allows = (session: Session, operation: string) =>
  session.mode === "admin" &&
  (session.scope?.includes("*") || session.scope?.includes(operation));
export type Bucket = { id: string; globalAliases: string[]; created: string };
export type Key = { id: string; name: string; expired: boolean };
export type Health = {
  status: string;
  connectedNodes: number;
  knownNodes: number;
  storageNodesUp: number;
  storageNodes: number;
  partitionsAllOk: number;
  partitionsQuorum: number;
  partitions: number;
};
export type Grant = {
  accessKeyId: string;
  permissions: { read: boolean; write: boolean; owner: boolean };
};
export type LifecycleRule = {
  ID?: string;
  Status: "Enabled" | "Disabled";
  Filter?: { Prefix?: string };
  Expiration?: { Days: number };
  AbortIncompleteMultipartUpload?: { DaysAfterInitiation: number };
};
export type Details = {
  id: string;
  keys: Grant[];
  globalAliases: string[];
  quotas?: { maxSize: number | null; maxObjects: number | null };
  websiteAccess?: boolean;
  websiteConfig?: { indexDocument: string; errorDocument?: string | null };
  lifecycleRules?: LifecycleRule[];
  objects?: number;
  bytes?: number;
};
export type Page =
  "overview" | "cluster" | "buckets" | "keys" | "tokens" | "profile";

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
    if (payload && typeof payload === "object" && !Array.isArray(payload)) {
      const problem = payload as Record<string, unknown>;
      const message = [problem.error, problem.message].find(
        (value): value is string =>
          typeof value === "string" && value.trim() !== "",
      );
      const details = problem.problems;
      const fields =
        details && typeof details === "object" && !Array.isArray(details)
          ? Object.entries(details)
              .filter(
                (entry): entry is [string, string] =>
                  typeof entry[1] === "string" && entry[1].trim() !== "",
              )
              .map(
                ([field, detail]) => `${field.replaceAll("_", " ")}: ${detail}`,
              )
          : [];
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

export async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(new URL(path, document.baseURI), {
      ...options,
      headers: { "Content-Type": "application/json", ...options?.headers },
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
export const get = <T>(op: string, query = "") =>
  request<T>(`api/garage/${op}${query}`);
export const post = <T>(op: string, data: unknown = {}, query = "") =>
  request<T>(`api/garage/${op}${query}`, {
    method: "POST",
    body: JSON.stringify(data),
  });
export const short = (id: string) =>
  id.length > 18 ? `${id.slice(0, 14)}…` : id;
