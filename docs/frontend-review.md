# Frontend review

The frontend keeps Garage authorization on the server while presenting controls appropriate to the current credential's scopes. Components own view state; hooks own cancellable data loading; `api.ts` handles console requests; `s3.ts` handles direct, signed browser-to-Garage transfers. Shared formatting and navigation decisions belong under `lib/`.

## Correctness fixes

- Console requests preserve `Headers` objects, tuple headers, and caller content types.
- Superseded workspace and bucket-detail requests cannot replace newer results or reopen a dismissed bucket.
- Object previews are cancelled when changing folders, changing selection, or unmounting.
- Cancelled uploads reject before constructing an XHR, including when cancellation happens during signing.
- Dropped files cannot change the queue while a transfer owns its snapshot. Successful batches refresh the listing once.
- Access-key expiration inputs display local time while preserving the stored timestamp.
- Cluster status and layout requests respect their separate Garage scopes, and a failed response does not discard the other successful response.
- Alias edits, grant changes, and access-key edits use their corresponding operation scopes in the UI.
- Pending theme changes cannot apply colors after the authenticated workspace unmounts; changing themes clears obsolete color tokens.
- Outstanding confirmation promises settle when their provider unmounts.
- Usage loading validates complete counters and limits concurrent requests to six. Partial data never appears as a complete aggregate.
- Clipboard failures remain visible in the token-secret dialog.

## Responsibility and readability changes

- Workspace summary loading and bucket usage loading have dedicated hooks.
- Bucket permission changes have their own component and a shared authoritative refresh path.
- Larger form and click handlers are named functions outside JSX.
- Preview rendering uses a dedicated component instead of a chain of nested conditional expressions.
- `BucketDetails`, `AccessKey`, `WorkspacePage`, and cluster-specific type names replace ambiguous names.
- Byte formatting has explicit decimal and binary modes in one utility; UI components no longer import formatting from other views or the S3 transport module.
- Workspace headings and navigation checks use shared helpers.
- Multipart uploads and copies share their completion-document encoding.
- Unused login props, forwarding helpers, repeated key lookups, and obsolete CSS selectors were removed.
- The formatting target now includes TSX files.

## Documentation and verification

Named functions, hooks, data types, component prop fields, and nested type fields have adjacent one-line descriptions. Short inline rendering and event-forwarding callbacks stay beside the expressions they explain; substantive callbacks are extracted into documented functions.

`npm run check:docs` checks description coverage using the existing TypeScript parser shipped with Prettier. Unit tests are excluded.

Run `npm test` for error parsing, transfer cleanup, UI flows, and frontend regression coverage. Run `npm run build` for strict TypeScript checking and the production bundle. `E2E_DATABASE=sqlite sh dev/run-e2e.sh` verifies the application against an isolated Garage instance; add `E2E_LARGE_UPLOAD=1` to include the 257 MiB multipart scenarios.

The static stylesheet cleanup removes rules whose required classes no longer appear in the frontend. Existing base/theme cascade rules remain in place to preserve the current appearance.
