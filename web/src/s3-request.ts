/** isSameOrigin distinguishes authenticated backend requests from public S3 requests. */
function isSameOrigin(address: string): boolean {
  return (
    new URL(address, window.location.href).origin === window.location.origin
  );
}

/** isCancelled preserves cancellation without turning it into a CORS/network error. */
function isCancelled(cause: unknown, signal?: AbortSignal | null): boolean {
  if (signal?.aborted) return true;
  return (
    (cause instanceof Error || cause instanceof DOMException) &&
    cause.name === "AbortError"
  );
}

/** s3AccessError explains browser-blocked S3 requests without exposing signed URL secrets. */
export function s3AccessError(address: string, cause?: unknown): Error {
  const endpoint = new URL(address, window.location.href).origin;
  if (isSameOrigin(address))
    return new Error(
      "Could not connect to the Web UI. Check your connection and try again.",
      { cause },
    );
  return new Error(
    `The browser could not access Garage S3 at ${endpoint}. ` +
      `Bucket CORS settings may be blocking this request. Ask a Garage administrator to allow ` +
      `${window.location.origin} in the bucket's CORS rules for the required methods and headers. ` +
      `Also check endpoint connectivity and TLS certificates. The browser does not distinguish CORS blocks from network failures.`,
    { cause },
  );
}

/** fetchS3 adds actionable CORS guidance while preserving HTTP responses and cancellation. */
export async function fetchS3(
  address: string,
  options?: RequestInit,
): Promise<Response> {
  try {
    return await fetch(address, {
      ...options,
      credentials: isSameOrigin(address) ? "same-origin" : "omit",
    });
  } catch (cause) {
    if (isCancelled(cause, options?.signal)) throw cause;
    throw s3AccessError(address, cause);
  }
}
