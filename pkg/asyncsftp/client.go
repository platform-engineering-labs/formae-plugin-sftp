// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package asyncsftp wraps the pkg/sftp library with an async interface.
// Operations return immediately with an operation ID that can be polled for completion.
package asyncsftp

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Client wraps an SFTP connection with async operation support.
type Client struct {
	sftpClient *sftp.Client
	sshClient  *ssh.Client

	mu         sync.RWMutex
	operations map[string]*Operation
}

// Config holds connection settings.
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
}

// NewClient creates a new async SFTP client.
func NewClient(cfg Config) (*Client, error) {
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	sshConfig := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For development only
		Timeout:         10 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("sftp client failed: %w", err)
	}

	return &Client{
		sftpClient: sftpClient,
		sshClient:  sshClient,
		operations: make(map[string]*Operation),
	}, nil
}

// Close closes the SFTP and SSH connections.
func (c *Client) Close() error {
	var errs []error
	if c.sftpClient != nil {
		if err := c.sftpClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.sshClient != nil {
		if err := c.sshClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// =============================================================================
// Async Operations
// =============================================================================

// StartUpload begins uploading content to a file.
// Returns an operation ID to poll for completion.
func (c *Client) StartUpload(path string, content string, permissions os.FileMode) string {
	opID := uuid.New().String()

	op := &Operation{
		ID:        opID,
		Type:      OperationTypeUpload,
		Path:      path,
		State:     StateInProgress,
		StartedAt: time.Now(),
	}

	c.mu.Lock()
	c.operations[opID] = op
	c.mu.Unlock()

	go c.doUpload(op, content, permissions)

	return opID
}

// StartDelete begins deleting a file.
// Returns an operation ID to poll for completion.
func (c *Client) StartDelete(path string) string {
	opID := uuid.New().String()

	op := &Operation{
		ID:        opID,
		Type:      OperationTypeDelete,
		Path:      path,
		State:     StateInProgress,
		StartedAt: time.Now(),
	}

	c.mu.Lock()
	c.operations[opID] = op
	c.mu.Unlock()

	go c.doDelete(op)

	return opID
}

// GetStatus returns the current status of an operation.
func (c *Client) GetStatus(operationID string) (*Operation, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	op, ok := c.operations[operationID]
	if !ok {
		return nil, fmt.Errorf("operation not found: %s", operationID)
	}

	// Return a copy to avoid race conditions
	return op.Copy(), nil
}

// =============================================================================
// Synchronous Operations (for Read and simple operations)
// =============================================================================

// ReadFile reads a file and returns its contents and metadata.
func (c *Client) ReadFile(path string) (*FileInfo, error) {
	// Get file info
	stat, err := c.sftpClient.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("stat failed: %w", err)
	}

	// Read content
	f, err := c.sftpClient.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	defer func() { _ = f.Close() }()

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	return &FileInfo{
		Path:        path,
		Content:     string(content),
		Permissions: fmt.Sprintf("%04o", stat.Mode().Perm()),
		Size:        stat.Size(),
		ModifiedAt:  stat.ModTime(),
	}, nil
}

// SetPermissions changes file permissions (synchronous, fast operation).
func (c *Client) SetPermissions(path string, permissions os.FileMode) error {
	return c.sftpClient.Chmod(path, permissions)
}

// ListFiles returns all file paths in a directory.
func (c *Client) ListFiles(dir string) ([]string, error) {
	entries, err := c.sftpClient.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readdir failed: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			paths = append(paths, dir+"/"+entry.Name())
		}
	}
	return paths, nil
}

// =============================================================================
// Internal implementation
// =============================================================================

func (c *Client) doUpload(op *Operation, content string, permissions os.FileMode) {
	// Create/overwrite the file
	f, err := c.sftpClient.Create(op.Path)
	if err != nil {
		c.completeOperation(op, StateFailure, fmt.Errorf("create failed: %w", err))
		return
	}

	// Write content
	_, err = f.Write([]byte(content))
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		c.completeOperation(op, StateFailure, fmt.Errorf("write failed: %w", err))
		return
	}

	// Set permissions
	if err := c.sftpClient.Chmod(op.Path, permissions); err != nil {
		c.completeOperation(op, StateFailure, fmt.Errorf("chmod failed: %w", err))
		return
	}

	// Get final file info
	stat, err := c.sftpClient.Stat(op.Path)
	if err != nil {
		c.completeOperation(op, StateFailure, fmt.Errorf("stat failed: %w", err))
		return
	}

	op.Result = &FileInfo{
		Path:        op.Path,
		Content:     content,
		Permissions: fmt.Sprintf("%04o", stat.Mode().Perm()),
		Size:        stat.Size(),
		ModifiedAt:  stat.ModTime(),
	}

	c.completeOperation(op, StateCompleted, nil)
}

func (c *Client) doDelete(op *Operation) {
	err := c.sftpClient.Remove(op.Path)
	if err != nil {
		if os.IsNotExist(err) {
			// Already deleted - treat as success
			c.completeOperation(op, StateCompleted, nil)
			return
		}
		c.completeOperation(op, StateFailure, fmt.Errorf("remove failed: %w", err))
		return
	}

	c.completeOperation(op, StateCompleted, nil)
}

func (c *Client) completeOperation(op *Operation, state OperationState, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	op.State = state
	op.CompletedAt = time.Now()
	if err != nil {
		op.Error = err.Error()
	}
}
