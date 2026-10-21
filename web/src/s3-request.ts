/** s3AccessError explains browser-blocked S3 requests without exposing signed URL secrets. */
export function s3AccessError(address: string, cause?: unknown): Error {
  const endpoint = new URL(address, window.location.href).origin;
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
    return await fetch(address, options);
  } catch (cause) {
    if (
      options?.signal?.aborted ||
      ((cause instanceof Error || cause instanceof DOMException) &&
        cause.name === "AbortError")
    )
      throw cause;
    throw s3AccessError(address, cause);
  }
}
