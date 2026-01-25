// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package asyncsftp

import (
	"errors"
	"time"
)

// ErrNotFound indicates the file does not exist.
var ErrNotFound = errors.New("file not found")

// OperationState represents the state of an async operation.
type OperationState string

const (
	StateInProgress OperationState = "IN_PROGRESS"
	StateCompleted  OperationState = "COMPLETED"
	StateFailure    OperationState = "FAILURE"
)

// OperationType indicates what kind of operation this is.
type OperationType string

const (
	OperationTypeUpload OperationType = "UPLOAD"
	OperationTypeDelete OperationType = "DELETE"
)

// Operation represents an async SFTP operation.
type Operation struct {
	ID          string
	Type        OperationType
	Path        string
	State       OperationState
	Error       string
	Result      *FileInfo
	StartedAt   time.Time
	CompletedAt time.Time
}

// Copy returns a copy of the operation (to avoid race conditions).
func (o *Operation) Copy() *Operation {
	if o == nil {
		return nil
	}
	copy := *o
	if o.Result != nil {
		resultCopy := *o.Result
		copy.Result = &resultCopy
	}
	return &copy
}

// FileInfo contains file metadata and content.
type FileInfo struct {
	Path        string
	Content     string
	Permissions string // e.g., "0644"
	Size        int64
	ModifiedAt  time.Time
}
