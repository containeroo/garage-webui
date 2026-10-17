import { request } from "./api";
import { abortMultipart } from "./multipart";

export type S3Action =
  | "list"
  | "download"
  | "preview"
  | "delete"
  | "copy"
  | "multipart-start"
  | "multipart-part"
  | "multipart-complete"
  | "multipart-abort"
  | "multipart-copy-start"
  | "multipart-copy-part"
  | "multipart-copy-complete"
  | "multipart-copy-abort";
export type SignedOperation = {
  url: string;
  method: "GET" | "POST" | "PUT" | "DELETE";
  expiresIn: number;
  copySource?: string;
  copyHeaders?: Record<string, string>;
};
export type ObjectItem = { key: string; size: number; modified: string };
export type ObjectPage = {
  items: ObjectItem[];
  folders: string[];
  nextToken: string;
};
export type TransferProgress = {
  loaded: number;
  total: number;
  speed: number;
  eta: number | null;
};
export type OperationInput = {
  key?: string;
  prefix?: string;
  sourceKey?: string;
  continuationToken?: string;
  uploadId?: string;
  partNumber?: number;
  rangeStart?: number;
  rangeEnd?: number;
};

const PART_THRESHOLD = 256 * 1024 ** 2;
const MAX_OBJECT = 5 * 1024 ** 4;
const MAX_PARTS = 10000;
const MIN_PART = 16 * 1024 ** 2;

// signOperation delegates authentication and S3 signing to the Web UI backend.
export function signOperation(
  bucketId: string,
  action: S3Action,
  input: OperationInput = {},
  signal?: AbortSignal,
): Promise<SignedOperation> {
  return request<SignedOperation>("api/objects/presign", {
    method: "POST",
    body: JSON.stringify({ bucketId, action, ...input }),
    signal,
  });
}

// nodeText looks up an XML field without relying on the Garage XML namespace.
export function nodeText(node: Element, name: string): string {
  return node.getElementsByTagNameNS("*", name)[0]?.textContent || "";
}

// parseXML reports malformed responses, including S3's error documents.
export function parseXML(xml: string): Document {
  const document = new DOMParser().parseFromString(xml, "application/xml");
  if (
    document.querySelector("parsererror") ||
    document.getElementsByTagNameNS("*", "Error").length > 0
  ) {
    throw new Error(
      nodeText(document.documentElement, "Message") ||
        "Garage returned an invalid S3 response",
    );
  }
  return document;
}

// parseListPage parses a single paginated folder listing (not a full-bucket scan).
export function parseListPage(xml: string): ObjectPage {
  const document = parseXML(xml);
  const decode = (text: string) => decodeURIComponent(text);
  const items = Array.from(
    document.getElementsByTagNameNS("*", "Contents"),
  ).map((node) => ({
    key: decode(nodeText(node, "Key")),
    size: Number(nodeText(node, "Size")),
    modified: nodeText(node, "LastModified"),
  }));
  const folders = Array.from(
    document.getElementsByTagNameNS("*", "CommonPrefixes"),
  ).map((node) => decode(nodeText(node, "Prefix")));
  const truncated =
    nodeText(document.documentElement, "IsTruncated") === "true";
  const nextToken = truncated
    ? nodeText(document.documentElement, "NextContinuationToken")
    : "";
  if (truncated && !nextToken)
    throw new Error("Garage did not return a page token");
  return { items, folders, nextToken };
}

export async function listObjects(
  bucketId: string,
  prefix: string,
  continuationToken: string,
  signal?: AbortSignal,
): Promise<ObjectPage> {
  const signed = await signOperation(
    bucketId,
    "list",
    { prefix, continuationToken },
    signal,
  );
  const response = await fetch(signed.url, { credentials: "omit", signal });
  if (!response.ok)
    throw new Error(`Garage could not list objects (HTTP ${response.status})`);
  return parseListPage(await response.text());
}

// transfer is the shared browser-to-Garage request primitive for all file uploads.
function transfer(
  signed: SignedOperation,
  body: Blob,
  signal: AbortSignal,
  onProgress: (bytes: number) => void,
  headers: Record<string, string> = {},
): Promise<string> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    const finish = () => signal.removeEventListener("abort", abort);
    xhr.open(signed.method, signed.url);
    xhr.withCredentials = false;
    for (const [name, value] of Object.entries(headers))
      xhr.setRequestHeader(name, value);
    xhr.upload.onprogress = (event) => onProgress(event.loaded);
    xhr.onload = () => {
      finish();
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(`Garage rejected the upload (HTTP ${xhr.status})`));
      } else {
        onProgress(body.size);
        resolve(xhr.getResponseHeader("ETag") || "");
      }
    };
    xhr.onerror = () => {
      finish();
      reject(new Error("Cannot reach the Garage S3 endpoint"));
    };
    xhr.onabort = () => {
      finish();
      reject(new DOMException("Upload cancelled", "AbortError"));
    };
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) abort();
    else xhr.send(body);
  });
}

// partSize adapts to the 10,000-part S3 limit without keeping file data in memory.
export function partSize(size: number): number {
  return Math.max(
    MIN_PART,
    Math.ceil(size / MAX_PARTS / 1024 ** 2) * 1024 ** 2,
  );
}

// uploadObject switches to multipart for large files and reports total transfer bytes.
export async function uploadObject(
  bucketId: string,
  key: string,
  file: File,
  signal: AbortSignal,
  onProgress: (loaded: number) => void,
): Promise<void> {
  if (!file.size || file.size > MAX_OBJECT)
    throw new Error("File must be between 1 byte and 5 TiB");
  if (file.size <= PART_THRESHOLD) {
    const signed = await request<SignedOperation>("api/uploads/presign", {
      method: "POST",
      body: JSON.stringify({ bucketId, key, size: file.size }),
      signal,
    });
    await transfer(
      signed,
      file,
      signal,
      onProgress,
      file.type ? { "Content-Type": file.type } : {},
    );
    return;
  }
  const begin = await signOperation(
    bucketId,
    "multipart-start",
    { key },
    signal,
  );
  const created = await fetch(begin.url, {
    method: begin.method,
    credentials: "omit",
    signal,
  });
  if (!created.ok)
    throw new Error(
      `Garage could not start multipart upload (${created.status})`,
    );
  const uploadId = nodeText(
    parseXML(await created.text()).documentElement,
    "UploadId",
  );
  if (!uploadId) throw new Error("Garage returned no multipart upload ID");
  const parts: { number: number; etag: string }[] = [];
  const chunk = partSize(file.size);
  try {
    for (
      let start = 0, number = 1;
      start < file.size;
      start += chunk, number++
    ) {
      const data = file.slice(start, Math.min(file.size, start + chunk));
      const signed = await signOperation(
        bucketId,
        "multipart-part",
        { key, uploadId, partNumber: number },
        signal,
      );
      const etag = await transfer(signed, data, signal, (loaded) =>
        onProgress(start + loaded),
      );
      if (!etag)
        throw new Error(
          "Garage did not expose the multipart ETag; check bucket CORS",
        );
      parts.push({ number, etag });
    }
    const xml = `<CompleteMultipartUpload>${parts.map(({ number, etag }) => `<Part><PartNumber>${number}</PartNumber><ETag>${escapeXML(etag)}</ETag></Part>`).join("")}</CompleteMultipartUpload>`;
    const signed = await signOperation(
      bucketId,
      "multipart-complete",
      { key, uploadId },
      signal,
    );
    const response = await fetch(signed.url, {
      method: "POST",
      body: xml,
      credentials: "omit",
      signal,
    });
    if (!response.ok)
      throw new Error(
        `Garage could not complete upload (HTTP ${response.status})`,
      );
    parseXML(await response.text());
    onProgress(file.size);
  } catch (error) {
    await abortMultipart(async () => {
      const cleanupSignal = AbortSignal.timeout(10000);
      const abort = await signOperation(
        bucketId,
        "multipart-abort",
        {
          key,
          uploadId,
        },
        cleanupSignal,
      );
      return fetch(abort.url, {
        method: "DELETE",
        credentials: "omit",
        signal: cleanupSignal,
      });
    }, error);
  }
}

// escapeXML escapes values returned by Garage before placing them in completion XML.
function escapeXML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

export async function deleteObject(
  bucketId: string,
  key: string,
): Promise<void> {
  const signed = await signOperation(bucketId, "delete", { key });
  const response = await fetch(signed.url, {
    method: "DELETE",
    credentials: "omit",
  });
  if (!response.ok)
    throw new Error(
      `Garage could not delete “${key}” (HTTP ${response.status})`,
    );
}

// copyObject uses server-side S3 copy without downloading object bytes to the browser.
export async function copyObject(
  bucketId: string,
  sourceKey: string,
  key: string,
  size: number,
): Promise<void> {
  if (size < 0 || size > MAX_OBJECT)
    throw new Error("Unsupported source object size");
  if (size <= 5 * 1024 ** 3) {
    const signed = await signOperation(bucketId, "copy", { sourceKey, key });
    const response = await fetch(signed.url, {
      method: "PUT",
      credentials: "omit",
      headers: { "x-amz-copy-source": signed.copySource || "" },
    });
    if (!response.ok)
      throw new Error(
        `Garage could not copy “${sourceKey}” (HTTP ${response.status})`,
      );
    parseXML(await response.text());
    return;
  }
  await copyMultipart(bucketId, sourceKey, key, size);
}

// copyMultipart delegates byte-range copies to Garage, completing only after all parts succeed.
async function copyMultipart(
  bucketId: string,
  sourceKey: string,
  key: string,
  size: number,
): Promise<void> {
  const input = { sourceKey, key };
  const begin = await signOperation(bucketId, "multipart-copy-start", input);
  const created = await fetch(begin.url, {
    method: "POST",
    credentials: "omit",
  });
  if (!created.ok)
    throw new Error(`Cannot initiate S3 copy (${created.status})`);
  const uploadId = nodeText(
    parseXML(await created.text()).documentElement,
    "UploadId",
  );
  if (!uploadId) throw new Error("Garage returned no copy upload ID");
  const parts: { number: number; etag: string }[] = [];
  const chunk = partSize(size);
  try {
    for (let start = 0, number = 1; start < size; start += chunk, number++) {
      const signed = await signOperation(bucketId, "multipart-copy-part", {
        ...input,
        uploadId,
        partNumber: number,
        rangeStart: start,
        rangeEnd: Math.min(size, start + chunk) - 1,
      });
      const response = await fetch(signed.url, {
        method: "PUT",
        credentials: "omit",
        headers: signed.copyHeaders || {},
      });
      if (!response.ok)
        throw new Error(`Cannot copy part ${number} (${response.status})`);
      const etag = nodeText(
        parseXML(await response.text()).documentElement,
        "ETag",
      );
      if (!etag) throw new Error("Garage did not return a copy-part ETag");
      parts.push({ number, etag });
    }
    const body = `<CompleteMultipartUpload>${parts.map(({ number, etag }) => `<Part><PartNumber>${number}</PartNumber><ETag>${escapeXML(etag)}</ETag></Part>`).join("")}</CompleteMultipartUpload>`;
    const finish = await signOperation(bucketId, "multipart-copy-complete", {
      ...input,
      uploadId,
    });
    const response = await fetch(finish.url, {
      method: "POST",
      body,
      credentials: "omit",
    });
    if (!response.ok)
      throw new Error(`Cannot complete S3 copy (${response.status})`);
    parseXML(await response.text());
  } catch (cause) {
    await abortMultipart(async () => {
      const cleanupSignal = AbortSignal.timeout(10000);
      const abort = await signOperation(
        bucketId,
        "multipart-copy-abort",
        {
          ...input,
          uploadId,
        },
        cleanupSignal,
      );
      return fetch(abort.url, {
        method: "DELETE",
        credentials: "omit",
        signal: cleanupSignal,
      });
    }, cause);
  }
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  if (value < 1024) return `${Math.round(value)} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)) - 1,
    units.length - 1,
  );
  return `${(value / 1024 ** (index + 1)).toFixed(1)} ${units[index]}`;
}
