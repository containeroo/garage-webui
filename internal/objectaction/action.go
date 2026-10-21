// Package objectaction defines the browser object operations shared by validation and S3 signing.
package objectaction

import "net/http"

// Action identifies one authorized object operation using its existing JSON representation.
type Action string

const (
	// Upload sends a complete object in a single request.
	Upload Action = "upload"
	// List retrieves a page of object metadata and folder prefixes.
	List Action = "list"
	// Download retrieves an object as an attachment.
	Download Action = "download"
	// Preview retrieves an object for inline display.
	Preview Action = "preview"
	// Delete removes one object.
	Delete Action = "delete"
	// Copy copies an object within its bucket.
	Copy Action = "copy"
	// MultipartStart creates a multipart upload.
	MultipartStart Action = "multipart-start"
	// MultipartPart uploads one numbered part.
	MultipartPart Action = "multipart-part"
	// MultipartComplete commits the uploaded parts.
	MultipartComplete Action = "multipart-complete"
	// MultipartAbort removes an incomplete multipart upload.
	MultipartAbort Action = "multipart-abort"
	// MultipartCopyStart creates an upload for a large server-side copy.
	MultipartCopyStart Action = "multipart-copy-start"
	// MultipartCopyPart copies one byte range into an upload part.
	MultipartCopyPart Action = "multipart-copy-part"
	// MultipartCopyComplete commits the copied parts.
	MultipartCopyComplete Action = "multipart-copy-complete"
	// MultipartCopyAbort removes an incomplete multipart copy.
	MultipartCopyAbort Action = "multipart-copy-abort"
)

// Method returns the S3 request method and rejects unknown or empty actions.
func (action Action) Method() (string, bool) {
	switch action {
	case List, Download, Preview:
		return http.MethodGet, true
	case Upload, Copy, MultipartPart, MultipartCopyPart:
		return http.MethodPut, true
	case Delete, MultipartAbort, MultipartCopyAbort:
		return http.MethodDelete, true
	case MultipartStart, MultipartComplete, MultipartCopyStart, MultipartCopyComplete:
		return http.MethodPost, true
	default:
		return "", false
	}
}
