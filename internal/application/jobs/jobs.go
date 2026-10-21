// Package jobs runs durable Garage object deletion jobs independently of browser sessions.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/auth"
	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/objectpath"
	"github.com/containeroo/garage-webui/internal/storage"
)

const (
	// leaseLength bounds takeover time after a crashed process.
	leaseLength = time.Minute
	// jobRetention retains terminal job metadata for a week.
	jobRetention = 7 * 24 * time.Hour
	// progressBatchSize limits database writes during large recursive deletions.
	progressBatchSize int64 = 100
)

// Selection contains the confirmed files and folder prefixes to delete.
type Selection struct {
	// Files are exact object keys.
	Files []string `json:"files"`
	// Folders are recursive object-key prefixes ending with /.
	Folders []string `json:"folders"`
}

// ObjectServices prepares backend S3 operations from independently verified credentials.
type ObjectServices interface {
	ForIdentity(auth.Identity) (*objects.Service, error)
}

// Service manages persistent jobs and processes them through credential-scoped Garage signing.
type Service struct {
	// store provides atomic job claiming and durable progress.
	store storage.DeletionJobs
	// authentication encrypts credentials and revalidates Garage permissions.
	authentication *auth.Service
	// objects prepares S3 requests with the initiating credential.
	objects ObjectServices
	// client performs trusted backend-to-Garage HTTP requests.
	client *http.Client
	// logger reports safe operator diagnostics.
	logger *slog.Logger
	// worker is a random unique lease owner per instance.
	worker string
}

// New constructs a job coordinator without starting background goroutines.
func New(
	store storage.DeletionJobs,
	authentication *auth.Service,
	objects ObjectServices,
	logger *slog.Logger,
) (*Service, error) {
	if store == nil || authentication == nil || objects == nil {
		return nil, errors.New("deletion jobs require storage, authentication and object services")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	return &Service{
		store:          store,
		authentication: authentication,
		objects:        objects,
		logger:         logger,
		worker:         hex.EncodeToString(random[:]),
		client:         &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// validFolder forbids root deletion, traversal, ambiguous separators and invalid prefixes.
func validFolder(prefix string) bool {
	return strings.HasSuffix(prefix, "/") && objectpath.ValidDeleteObjectKey(prefix)
}

// ValidateSelection rejects malformed or dangerously broad requests before a job is created.
func ValidateSelection(sel Selection) error {
	if len(sel.Files)+len(sel.Folders) == 0 || len(sel.Files)+len(sel.Folders) > 200 {
		return errors.New("select between 1 and 200 files or folders")
	}

	for _, f := range sel.Folders {
		if !validFolder(f) {
			return errors.New("invalid folder prefix")
		}
	}

	for _, k := range sel.Files {
		if !objectpath.ValidDeleteObjectKey(k) {
			return errors.New("invalid object key")
		}
	}
	return nil
}

// Start persists a validated, credential-authorized deletion job before returning to the caller.
func (s *Service) Start(
	ctx context.Context, id auth.Identity, bucket string, sel Selection,
) (storage.DeletionJob, error) {
	if bucket == "" || len(bucket) > 256 {
		return storage.DeletionJob{}, errors.New("invalid bucket")
	}
	if err := ValidateSelection(sel); err != nil {
		return storage.DeletionJob{}, err
	}

	// Check that the initiating credential can resolve the bucket; Garage still enforces each S3 DELETE.
	service, err := s.objects.ForIdentity(id)
	if err != nil {
		return storage.DeletionJob{}, err
	}
	prefix := ""
	if len(sel.Folders) > 0 {
		prefix = sel.Folders[0]
	}
	if _, err = service.Prepare(ctx, id.Credential.Kind,
		objects.Request{
			BucketID: bucket,
			Action:   objectaction.List,
			Prefix:   prefix,
		}, time.Now()); err != nil {
		return storage.DeletionJob{}, err
	}

	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return storage.DeletionJob{}, err
	}
	jobID := hex.EncodeToString(random[:])

	owner, err := s.authentication.JobOwner(id.Credential)
	if err != nil {
		return storage.DeletionJob{}, err
	}
	encrypted, err := s.authentication.EncryptJobCredential(jobID, id.Credential)
	if err != nil {
		return storage.DeletionJob{}, err
	}
	targets, err := json.Marshal(sel)
	if err != nil {
		return storage.DeletionJob{}, err
	}

	job := storage.DeletionJob{
		ID:                  jobID,
		Owner:               owner,
		EncryptedCredential: encrypted,
		Bucket:              bucket,
		Targets:             string(targets),
		Status:              "pending",
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	if err = s.store.CreateDeletionJob(ctx, job); err != nil {
		return storage.DeletionJob{}, err
	}
	return job, nil
}

// List returns current jobs that match the caller's independently verified Garage credential.
func (s *Service) List(ctx context.Context, id auth.Identity) ([]storage.DeletionJob, error) {
	owner, err := s.authentication.JobOwner(id.Credential)
	if err != nil {
		return nil, err
	}
	found, err := s.store.ListDeletionJobs(ctx, owner)
	if err != nil {
		return nil, err
	}

	for i := range found {
		var selection Selection
		if json.Unmarshal([]byte(found[i].Targets), &selection) != nil {
			continue
		}
		if len(selection.Folders) > 0 {
			found[i].Target = selection.Folders[0]
		} else if len(selection.Files) > 0 {
			found[i].Target = selection.Files[0]
		}
		if len(selection.Folders)+len(selection.Files) > 1 {
			found[i].Target += " (+ others)"
		}
	}

	return found, nil
}

// Cancel restricts cancellation to jobs created with this exact Garage credential.
func (s *Service) Cancel(ctx context.Context, id auth.Identity, jobID string) (bool, error) {
	owner, err := s.authentication.JobOwner(id.Credential)
	if err != nil {
		return false, err
	}
	return s.store.CancelDeletionJob(ctx, jobID, owner)
}

// Cleanup removes terminal jobs after the configured retention interval.
func (s *Service) Cleanup(ctx context.Context) error {
	return s.store.CleanupDeletionJobs(ctx, time.Now().Add(-jobRetention))
}

// Run polls the database and claims one job at a time; restart and replica recovery require no memory state.
func (s *Service) Run(ctx context.Context) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()

	for {
		if ctx.Err() != nil {
			return
		}
		job, ok, err := s.store.ClaimDeletionJob(ctx, s.worker, leaseLength)
		if err != nil {
			s.logger.Warn("claim deletion job", "error", err)
		} else if ok {
			s.execute(ctx, job)
		}

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

// execute renews the worker lease while authorizing and deleting objects.
func (s *Service) execute(parent context.Context, job storage.DeletionJob) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	heartbeatDone := make(chan struct{})
	var cancelRequested atomic.Bool
	var leaseLost atomic.Bool
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, done := context.WithTimeout(parent, 5*time.Second)
				cancelled, owned, err := s.store.RenewDeletionJob(checkCtx, job.ID, s.worker, leaseLength)
				done()
				if err != nil || !owned {
					leaseLost.Store(true)
					if err != nil {
						s.logger.Warn("renew deletion job", "error", err)
					}
					cancel()
					return
				}
				if cancelled {
					cancelRequested.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	executor := &execution{service: s}
	var err error
	if job.CancelRequested {
		cancelRequested.Store(true)
		err = context.Canceled
	} else {
		err = s.process(ctx, job, executor)
	}
	cancel()
	<-heartbeatDone

	if parent.Err() != nil || leaseLost.Load() {
		return
	} // Let another worker reclaim after shutdown.

	flushCtx, flushDone := context.WithTimeout(parent, 10*time.Second)
	if flushErr := executor.flushProgress(flushCtx, job); flushErr != nil {
		s.logger.Warn("record deletion progress", "job", job.ID, "error", flushErr)
		if err == nil {
			err = flushErr
		}
	}
	flushDone()

	status, message := "completed", ""
	if err != nil {
		status, message = "failed", "deletion could not finish; check server logs"
		if errors.Is(err, context.Canceled) && cancelRequested.Load() {
			status, message = "cancelled", ""
		}
		if errors.Is(err, auth.ErrUnauthenticated) {
			message = "Garage credential expired or was revoked"
		}
		s.logger.Warn("deletion job interrupted", "job", job.ID, "error", err)
	}

	finishCtx, done := context.WithTimeout(parent, 10*time.Second)
	defer done()
	if err := s.store.FinishDeletionJob(finishCtx, job.ID, s.worker, status, message); err != nil {
		s.logger.Warn("finish deletion job", "job", job.ID, "error", err)
	}
}

// process iterates exact objects and folder prefixes, rescanning pages until all nested keys are gone.
func (s *Service) process(ctx context.Context, job storage.DeletionJob, executor *execution) error {
	credential, err := s.authentication.DecryptJobCredential(job.ID, job.EncryptedCredential)
	if err != nil {
		return err
	}

	var sel Selection
	if err = json.Unmarshal([]byte(job.Targets), &sel); err != nil {
		return err
	}
	if err = ValidateSelection(sel); err != nil {
		return err
	}

	executor.credential = credential
	// Deleting a parent covers selected descendants; prefer one uniform recursive pass.
	for _, folder := range sel.Folders {
		if err := executor.deleteFolder(ctx, job, folder); err != nil {
			return err
		}
	}

	for _, key := range sel.Files {
		covered := false
		for _, folder := range sel.Folders {
			if strings.HasPrefix(key, folder) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if err := executor.deleteKey(ctx, job, key); err != nil {
			return err
		}
	}

	return nil
}

// execution reuses a scoped signer while periodically verifying the initiating Garage credential.
type execution struct {
	// service owns authentication and the shared database.
	service *Service
	// credential is decrypted only within the running worker.
	credential auth.Credential
	// signer is refreshed after credential revalidation.
	signer *objects.Service
	// recheck is the deadline for validating Garage authorization again.
	recheck time.Time
	// pendingProgress tracks successful deletions not yet persisted to the job row.
	pendingProgress int64
}

// flushProgress persists accumulated successful deletions in one database update.
func (e *execution) flushProgress(ctx context.Context, job storage.DeletionJob) error {
	if e.pendingProgress == 0 {
		return nil
	}
	if err := e.service.store.RecordDeletionProgress(ctx, job.ID, e.service.worker, e.pendingProgress); err != nil {
		return err
	}
	e.pendingProgress = 0
	return nil
}

// signedRequest periodically revalidates Garage authorization and performs one scoped S3 operation.
func (e *execution) signedRequest(ctx context.Context, bucket string, input objects.Request) (*http.Response, error) {
	if e.signer == nil || !time.Now().Before(e.recheck) {
		identity, err := e.service.authentication.Verify(ctx, e.credential)
		if err != nil {
			if errors.Is(err, auth.ErrUnauthenticated) {
				return nil, auth.ErrUnauthenticated
			}
			return nil, errors.New("garage credential verification failed")
		}
		signer, err := e.service.objects.ForIdentity(identity)
		if err != nil {
			return nil, errors.New("garage object service unavailable")
		}
		e.signer = signer
		e.recheck = time.Now().Add(10 * time.Second)
	}
	signed, err := e.signer.Prepare(ctx, e.credential.Kind, input, time.Now())
	if err != nil {
		return nil, errors.New("cannot prepare Garage object operation")
	}
	request, err := http.NewRequestWithContext(ctx, signed.Method, signed.URL, nil)
	if err != nil {
		return nil, err
	}
	response, err := e.service.client.Do(request)
	if err != nil {
		return nil, errors.New("garage S3 transport request failed")
	}
	return response, nil
}

// listFirstPage reads one bounded S3 page without exposing signed URLs to logs.
func (e *execution) listFirstPage(ctx context.Context, bucket, prefix string) ([]string, []string, error) {
	response, err := e.signedRequest(ctx, bucket, objects.Request{BucketID: bucket, Action: objectaction.List, Prefix: prefix})
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close() // nolint:errcheck
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("garage list returned HTTP %d", response.StatusCode)
	}
	var page struct {
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
		Prefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&page); err != nil {
		return nil, nil, err
	}

	keys := make([]string, 0, len(page.Contents))
	children := make([]string, 0, len(page.Prefixes))
	// Garage requests encoding-type=url. PathUnescape does not turn literal + into spaces.
	for _, item := range page.Contents {
		key, err := url.PathUnescape(item.Key)
		if err != nil || !strings.HasPrefix(key, prefix) || !objectpath.ValidDeleteObjectKey(key) {
			return nil, nil, errors.New("invalid S3 listing key")
		}
		keys = append(keys, key)
	}

	for _, item := range page.Prefixes {
		child, err := url.PathUnescape(item.Prefix)
		if err != nil || !validFolder(child) || !strings.HasPrefix(child, prefix) || child == prefix {
			return nil, nil, errors.New("invalid S3 listing folder")
		}
		children = append(children, child)
	}

	return keys, children, nil
}

// deleteFolder repeatedly scans the first page to avoid invalid continuation tokens after mutations.
func (e *execution) deleteFolder(ctx context.Context, job storage.DeletionJob, folder string) error {
	stack := []string{folder}

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		prefix := stack[len(stack)-1]
		keys, children, err := e.listFirstPage(ctx, job.Bucket, prefix)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := e.deleteKey(ctx, job, key); err != nil {
				return err
			}
		}
		if len(children) > 0 {
			// Descend into a child before revisiting its parent; no unbounded in-memory key list.
			stack = append(stack, children[0])
			continue
		}
		if len(keys) == 0 {
			stack = stack[:len(stack)-1]
		}
	}
	return nil
}

// deleteKey signs and performs an S3 deletion before batching its persisted progress increment.
func (e *execution) deleteKey(ctx context.Context, job storage.DeletionJob, key string) error {
	response, err := e.signedRequest(ctx, job.Bucket, objects.Request{BucketID: job.Bucket, Action: objectaction.Delete, Key: key})
	if err != nil {
		return err
	}
	defer response.Body.Close() // nolint:errcheck
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		return fmt.Errorf("garage delete returned HTTP %d", response.StatusCode)
	}
	e.pendingProgress++
	if e.pendingProgress < progressBatchSize {
		return nil
	}
	return e.flushProgress(ctx, job)
}
