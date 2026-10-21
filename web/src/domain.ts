/** Garage credential kinds accepted by the sign-in and authorization boundaries. */
export const CredentialKind = {
  /** A scoped Garage Admin API token. */
  Admin: "admin",
  /** An S3 access-key ID and secret. */
  S3: "s3",
} as const;
/** The supported Garage credential kinds, excluding an anonymous session. */
export type CredentialKind =
  (typeof CredentialKind)[keyof typeof CredentialKind];

/** Object actions shared by direct transfer requests and backend authorization. */
export const ObjectAction = {
  /** Sends an object in a single upload request. */
  Upload: "upload",
  /** Retrieves a page of object and folder metadata. */
  List: "list",
  /** Downloads an object as an attachment. */
  Download: "download",
  /** Retrieves an object for inline display. */
  Preview: "preview",
  /** Removes an object from its bucket. */
  Delete: "delete",
  /** Copies an object within its bucket. */
  Copy: "copy",
  /** Creates an upload for a large file. */
  MultipartStart: "multipart-start",
  /** Transfers one numbered upload part. */
  MultipartPart: "multipart-part",
  /** Commits uploaded part receipts. */
  MultipartComplete: "multipart-complete",
  /** Removes an incomplete multipart upload. */
  MultipartAbort: "multipart-abort",
  /** Creates an upload for a large server-side copy. */
  MultipartCopyStart: "multipart-copy-start",
  /** Copies one byte range into a multipart part. */
  MultipartCopyPart: "multipart-copy-part",
  /** Commits the copied part receipts. */
  MultipartCopyComplete: "multipart-copy-complete",
  /** Removes an incomplete multipart copy. */
  MultipartCopyAbort: "multipart-copy-abort",
} as const;
/** A supported browser object operation sent to the signing endpoint. */
export type ObjectAction = (typeof ObjectAction)[keyof typeof ObjectAction];

/** States in the sequential file-upload queue. */
export const UploadStatus = {
  /** The file has not been attempted. */
  Pending: "pending",
  /** The file currently owns the active transfer. */
  Uploading: "uploading",
  /** The file was uploaded successfully. */
  Done: "done",
  /** The file failed or was cancelled and may be retried. */
  Failed: "failed",
} as const;
/** The current state of one queued file. */
export type UploadStatus = (typeof UploadStatus)[keyof typeof UploadStatus];

/** Safe rendering strategies for an authorized object preview. */
export const PreviewKind = {
  /** Renders an image through the browser image element. */
  Image: "image",
  /** Renders a browser-supported video. */
  Video: "video",
  /** Renders a browser-supported audio file. */
  Audio: "audio",
  /** Fetches bounded text and renders it without interpreting markup. */
  Text: "text",
  /** Offers an external link for an unsupported inline format. */
  External: "external",
} as const;
/** The renderer selected for an object's preview. */
export type PreviewKind = (typeof PreviewKind)[keyof typeof PreviewKind];

/** Available views of an individual Garage bucket. */
export const BucketTab = {
  /** Bucket configuration and usage. */
  Overview: "overview",
  /** Access-key grants on the bucket. */
  Permissions: "permissions",
  /** Direct S3 object browsing and transfers. */
  Browse: "browse",
} as const;
/** The selected view inside a bucket. */
export type BucketTab = (typeof BucketTab)[keyof typeof BucketTab];
