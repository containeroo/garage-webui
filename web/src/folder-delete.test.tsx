// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { deleteFolder } from "./s3";

/** pageXML builds one representative Garage ListObjectsV2 page. */
function pageXML(
  keys: string[],
  folders: string[] = [],
  continuation = "",
): string {
  return `<ListBucketResult>
    ${keys.map((key) => `<Contents><Key>${key}</Key><Size>5</Size><LastModified>2026-01-01T00:00:00Z</LastModified></Contents>`).join("")}
    ${folders.map((folder) => `<CommonPrefixes><Prefix>${folder}</Prefix></CommonPrefixes>`).join("")}
    <IsTruncated>${continuation ? "true" : "false"}</IsTruncated>
    ${continuation ? `<NextContinuationToken>${continuation}</NextContinuationToken>` : ""}
  </ListBucketResult>`;
}

/** requestMetadata reads only the authenticated object operation's signed routing metadata. */
function requestMetadata(input: RequestInfo | URL) {
  const url = new URL(String(input));
  return JSON.parse(url.searchParams.get("request") || "{}");
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("recursively deletes nested folders after enumerating every listing page", async () => {
  const deleted: string[] = [];
  const visited: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const data = requestMetadata(input);
      if (options?.method === "DELETE") {
        deleted.push(data.key);
        return new Response(null, { status: 204 });
      }
      visited.push(`${data.prefix}:${data.continuationToken || ""}`);
      switch (`${data.prefix}:${data.continuationToken || ""}`) {
        case "photos/:":
          return new Response(
            pageXML(["photos/", "photos/top.txt"], ["photos/2025/"], "page-2"),
          );
        case "photos/:page-2":
          return new Response(pageXML(["photos/other.txt"], ["photos/2026/"]));
        case "photos/2025/:":
          return new Response(pageXML(["photos/2025/inside.jpg"]));
        case "photos/2026/:":
          return new Response(pageXML([], ["photos/2026/nested/"]));
        case "photos/2026/nested/:":
          return new Response(pageXML(["photos/2026/nested/file.txt"]));
        default:
          throw new Error(`unexpected listing: ${data.prefix}`);
      }
    }),
  );
  const progress = vi.fn();
  await expect(deleteFolder("bucket", "photos/", progress)).resolves.toBe(5);
  expect(deleted).toEqual([
    "photos/",
    "photos/top.txt",
    "photos/other.txt",
    "photos/2026/nested/file.txt",
    "photos/2025/inside.jpg",
  ]);
  expect(progress).toHaveBeenCalledTimes(5);
  expect(visited).toContain("photos/:page-2");
  expect(deleted).not.toContain("photo-album/outside.txt");
});

test("refuses root and ambiguous folder prefixes before any Garage requests", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  await expect(deleteFolder("bucket", "")).rejects.toThrow(
    "invalid or root folder",
  );
  await expect(deleteFolder("bucket", "photos")).rejects.toThrow(
    "invalid or root folder",
  );
  await expect(deleteFolder("bucket", "photos/../")).rejects.toThrow(
    "invalid or root folder",
  );
  await expect(deleteFolder("bucket", "photos//")).rejects.toThrow(
    "invalid or root folder",
  );
  expect(fetch).not.toHaveBeenCalled();
});

test("stops on a failed object delete without claiming recursive deletion succeeded", async () => {
  const deleted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const data = requestMetadata(input);
      if (options?.method === "DELETE") {
        deleted.push(data.key);
        return new Response(null, { status: 403 });
      }
      return new Response(pageXML(["work/a.txt", "work/b.txt"]));
    }),
  );
  await expect(deleteFolder("bucket", "work/")).rejects.toThrow("403");
  expect(deleted).toEqual(["work/a.txt"]);
});

test("rejects an incorrect listing prefix rather than deleting outside the folder", async () => {
  const fetch = vi.fn(async () => new Response(pageXML(["other/private.txt"])));
  vi.stubGlobal("fetch", fetch);
  await expect(deleteFolder("bucket", "work/")).rejects.toThrow(
    "outside the selected folder",
  );
  expect(fetch).toHaveBeenCalledTimes(1);
});

test("rejects repeated continuation tokens instead of looping indefinitely", async () => {
  const fetch = vi.fn(async () => new Response(pageXML([], [], "loop")));
  vi.stubGlobal("fetch", fetch);
  await expect(deleteFolder("bucket", "work/")).rejects.toThrow(
    "repeated listing continuation token",
  );
  expect(fetch).toHaveBeenCalledTimes(2);
});
