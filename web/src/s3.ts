import { fetchS3, s3AccessError } from "./s3-request";
import { ObjectAction } from "./domain";
import { request } from "./api";
import { abortMultipart } from "./multipart";

/** A short-lived S3 request authorized for one object operation. */
export type SignedOperation = {
  /** Short-lived authorized URL for this browser operation. */
  url: string;
  /** HTTP method that must be used with the signed URL. */
  method: "GET" | "POST" | "PUT" | "DELETE";
  /** Signed URL validity duration in seconds. */
  expiresIn: number;
  /** Encoded source object for a server-side copy. */
  copySource?: string;
  /** Signed headers required for a multipart copy request. */
  copyHeaders?: Record<string, string>;
};
/** An object's metadata from the current S3 listing page. */
export type ObjectItem = {
  /** Complete object key, including any folder prefix. */
  key: string;
  /** Object or file length in bytes. */
  size: number;
  /** Last modification timestamp returned by S3. */
  modified: string;
};
/** A bounded S3 listing with folders and an optional next-page token. */
export type ObjectPage = {
  /** Object metadata entries on the current listing page. */
  items: ObjectItem[];
  /** Common object-key prefixes representing child folders. */
  folders: string[];
  /** Opaque continuation token; empty indicates the last page. */
  nextToken: string;
};
/** The active file's byte progress, throughput, and remaining duration. */
export type TransferProgress = {
  /** Number of bytes transferred for the active file. */
  loaded: number;
  /** Total bytes expected for the active transfer. */
  total: number;
  /** Average transfer throughput in bytes per second. */
  speed: number;
  /** Estimated seconds remaining; null means insufficient progress. */
  eta: number | null;
};
/** Object identifiers and optional multipart parameters sent for signing. */
export type OperationInput = {
  /** Complete object key, including any folder prefix. */
  key?: string;
  /** Object-key prefix restricting the operation. */
  prefix?: string;
  /** Source object key for copy and move operations. */
  sourceKey?: string;
  /** Opaque S3 listing position from the preceding page. */
  continuationToken?: string;
  /** S3 identifier for the active multipart transfer. */
  uploadId?: string;
  /** One-based multipart part number. */
  partNumber?: number;
  /** Inclusive source byte offset for a copy part. */
  rangeStart?: number;
  /** Inclusive final source byte offset for a copy part. */
  rangeEnd?: number;
};

const PART_THRESHOLD = 256 * 1024 ** 2;
const MAX_OBJECT = 5 * 1024 ** 4;
const MAX_PARTS = 10000;
const MIN_PART = 16 * 1024 ** 2;

// signOperation delegates authentication and S3 signing to the Web UI backend.
export function signOperation(
  bucketId: string,
  action: ObjectAction,
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
  const items = Array.from(
    document.getElementsByTagNameNS("*", "Contents"),
  ).map((node) => ({
    key: decodeURIComponent(nodeText(node, "Key")),
    size: Number(nodeText(node, "Size")),
    modified: nodeText(node, "LastModified"),
  }));
  const folders = Array.from(
    document.getElementsByTagNameNS("*", "CommonPrefixes"),
  ).map((node) => decodeURIComponent(nodeText(node, "Prefix")));
  const truncated =
    nodeText(document.documentElement, "IsTruncated") === "true";
  const nextToken = truncated
    ? nodeText(document.documentElement, "NextContinuationToken")
    : "";
  if (truncated && !nextToken)
    throw new Error("garage did not return a page token");
  return { items, folders, nextToken };
}

/** listObjects fetches one signed S3 folder page directly from the browser. */
export async function listObjects(
  bucketId: string,
  prefix: string,
  continuationToken: string,
  signal?: AbortSignal,
): Promise<ObjectPage> {
  const signed = await signOperation(
    bucketId,
    ObjectAction.List,
    { prefix, continuationToken },
    signal,
  );
  const response = await fetchS3(signed.url, { credentials: "omit", signal });
  if (!response.ok)
    throw new Error(`garage could not list objects (HTTP ${response.status})`);
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
    if (signal.aborted) {
      reject(new DOMException("Upload cancelled", "AbortError"));
      return;
    }
    const xhr = new XMLHttpRequest();
    /** abort cancels the active browser upload request. */
    const abort = () => xhr.abort();
    /** finish resolves the pending prompt and releases its dialog state. */
    const finish = () => signal.removeEventListener("abort", abort);
    xhr.open(signed.method, signed.url);
    xhr.withCredentials = false;
    for (const [name, value] of Object.entries(headers))
      xhr.setRequestHeader(name, value);
    xhr.upload.onprogress = (event) => onProgress(event.loaded);
    xhr.onload = () => {
      finish();
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(`garage rejected the upload (HTTP ${xhr.status})`));
      } else {
        onProgress(body.size);
        resolve(xhr.getResponseHeader("ETag") || "");
      }
    };
    xhr.onerror = () => {
      finish();
      reject(s3AccessError(signed.url));
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
    throw new Error("file must be between 1 byte and 5 TiB");
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
    ObjectAction.MultipartStart,
    { key },
    signal,
  );
  const created = await fetchS3(begin.url, {
    method: begin.method,
    credentials: "omit",
    signal,
  });
  if (!created.ok)
    throw new Error(
      `garage could not start multipart upload (${created.status})`,
    );
  const uploadId = nodeText(
    parseXML(await created.text()).documentElement,
    "UploadId",
  );
  if (!uploadId) throw new Error("garage returned no multipart upload ID");
  const parts: {
    /** One-based part number in the completion sequence. */
    number: number;
    /** Garage receipt used to complete an uploaded or copied part. */
    etag: string;
  }[] = [];
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
        ObjectAction.MultipartPart,
        { key, uploadId, partNumber: number },
        signal,
      );
      const etag = await transfer(signed, data, signal, (loaded) =>
        onProgress(start + loaded),
      );
      if (!etag)
        throw new Error(
          "garage did not expose the multipart ETag; check bucket CORS",
        );
      parts.push({ number, etag });
    }
    const xml = completionXML(parts);
    const signed = await signOperation(
      bucketId,
      ObjectAction.MultipartComplete,
      { key, uploadId },
      signal,
    );
    const response = await fetchS3(signed.url, {
      method: "POST",
      body: xml,
      credentials: "omit",
      signal,
    });
    if (!response.ok)
      throw new Error(
        `garage could not complete upload (HTTP ${response.status})`,
      );
    parseXML(await response.text());
    onProgress(file.size);
  } catch (error) {
    await abortMultipart(async () => {
      const cleanupSignal = AbortSignal.timeout(10000);
      const abort = await signOperation(
        bucketId,
        ObjectAction.MultipartAbort,
        {
          key,
          uploadId,
        },
        cleanupSignal,
      );
      return fetchS3(abort.url, {
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

/** deleteObject requests authorization and deletes one S3 object directly. */
export async function deleteObject(
  bucketId: string,
  key: string,
): Promise<void> {
  const signed = await signOperation(bucketId, ObjectAction.Delete, { key });
  const response = await fetchS3(signed.url, {
    method: "DELETE",
    credentials: "omit",
  });
  if (!response.ok)
    throw new Error(
      `garage could not delete “${key}” (HTTP ${response.status})`,
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
    throw new Error("unsupported source object size");
  if (size <= 5 * 1024 ** 3) {
    const signed = await signOperation(bucketId, ObjectAction.Copy, {
      sourceKey,
      key,
    });
    const response = await fetchS3(signed.url, {
      method: "PUT",
      credentials: "omit",
      headers: { "x-amz-copy-source": signed.copySource || "" },
    });
    if (!response.ok)
      throw new Error(
        `garage could not copy “${sourceKey}” (HTTP ${response.status})`,
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
  const begin = await signOperation(
    bucketId,
    ObjectAction.MultipartCopyStart,
    input,
  );
  const created = await fetchS3(begin.url, {
    method: "POST",
    credentials: "omit",
  });
  if (!created.ok)
    throw new Error(`cannot initiate S3 copy (${created.status})`);
  const uploadId = nodeText(
    parseXML(await created.text()).documentElement,
    "UploadId",
  );
  if (!uploadId) throw new Error("garage returned no copy upload ID");
  const parts: {
    /** One-based part number in the completion sequence. */
    number: number;
    /** Garage receipt used to complete an uploaded or copied part. */
    etag: string;
  }[] = [];
  const chunk = partSize(size);
  try {
    for (let start = 0, number = 1; start < size; start += chunk, number++) {
      const signed = await signOperation(
        bucketId,
        ObjectAction.MultipartCopyPart,
        {
          ...input,
          uploadId,
          partNumber: number,
          rangeStart: start,
          rangeEnd: Math.min(size, start + chunk) - 1,
        },
      );
      const response = await fetchS3(signed.url, {
        method: "PUT",
        credentials: "omit",
        headers: signed.copyHeaders || {},
      });
      if (!response.ok)
        throw new Error(`cannot copy part ${number} (${response.status})`);
      const etag = nodeText(
        parseXML(await response.text()).documentElement,
        "ETag",
      );
      if (!etag) throw new Error("garage did not return a copy-part ETag");
      parts.push({ number, etag });
    }
    const body = completionXML(parts);
    const finish = await signOperation(
      bucketId,
      ObjectAction.MultipartCopyComplete,
      {
        ...input,
        uploadId,
      },
    );
    const response = await fetchS3(finish.url, {
      method: "POST",
      body,
      credentials: "omit",
    });
    if (!response.ok)
      throw new Error(`cannot complete S3 copy (${response.status})`);
    parseXML(await response.text());
  } catch (cause) {
    await abortMultipart(async () => {
      const cleanupSignal = AbortSignal.timeout(10000);
      const abort = await signOperation(
        bucketId,
        ObjectAction.MultipartCopyAbort,
        {
          ...input,
          uploadId,
        },
        cleanupSignal,
      );
      return fetchS3(abort.url, {
        method: "DELETE",
        credentials: "omit",
        signal: cleanupSignal,
      });
    }, cause);
  }
}

// completionXML encodes the ordered part receipts shared by multipart uploads and copies.
function completionXML(
  parts: {
    /** One-based part number in the completion sequence. */
    number: number;
    /** Garage receipt used to complete an uploaded or copied part. */
    etag: string;
  }[],
): string {
  const receipts = parts.map(
    ({ number, etag }) =>
      `<Part><PartNumber>${number}</PartNumber><ETag>${escapeXML(etag)}</ETag></Part>`,
  );
  return `<CompleteMultipartUpload>${receipts.join("")}</CompleteMultipartUpload>`;
}
