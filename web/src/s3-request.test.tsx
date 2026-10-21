// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { fetchS3, s3AccessError } from "./s3-request";

afterEach(() => vi.unstubAllGlobals());

test("explains CORS and network failures without leaking signed query parameters", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockRejectedValue(
        new TypeError("NetworkError when attempting to fetch resource."),
      ),
  );
  const signed =
    "https://s3.example.com/private/file?X-Amz-Signature=secret-signature";
  await expect(fetchS3(signed)).rejects.toThrow(
    "Bucket CORS settings may be blocking this request",
  );
  const error = s3AccessError(signed);
  expect(error.message).toContain("https://s3.example.com");
  expect(error.message).toContain(window.location.origin);
  expect(error.message).toContain("TLS");
  expect(error.message).not.toContain("secret-signature");
  expect(error.message).not.toContain("private/file");
});

test("preserves cancellation and HTTP permission failures", async () => {
  const aborted = new DOMException("cancelled", "AbortError");
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(aborted));
  await expect(fetchS3("https://s3.example.com")).rejects.toBe(aborted);
  const response = new Response("AccessDenied", { status: 403 });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
  expect(await fetchS3("https://s3.example.com")).toBe(response);
});
