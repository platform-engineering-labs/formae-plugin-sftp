// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"

	"github.com/platform-engineering-labs/formae-plugin-sftp/pkg/asyncsftp"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ErrNotImplemented is returned by stub methods that need implementation.
var ErrNotImplemented = errors.New("not implemented")

// =============================================================================
// Target Configuration
// =============================================================================

// TargetConfig holds SFTP target settings.
// Contains only the deployment location, NOT credentials.
// Credentials are provided via environment variables.
type TargetConfig struct {
	URL string `json:"url"` // sftp://host:port
}

// parseTargetConfig extracts SFTP target settings from the request.
func parseTargetConfig(data json.RawMessage) (*TargetConfig, error) {
	var cfg TargetConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid target config: %w", err)
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("target config missing 'url'")
	}
	return &cfg, nil
}

// parseURL extracts host and port from an SFTP URL.
// Expected format: sftp://host:port or sftp://host (defaults to port 22)
func parseURL(sftpURL string) (host string, port string, err error) {
	u, err := url.Parse(sftpURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "sftp" {
		return "", "", fmt.Errorf("expected sftp:// URL, got %s://", u.Scheme)
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		port = "22"
	}
	return host, port, nil
}

// getCredentials reads SFTP credentials from environment variables.
func getCredentials() (username, password string, err error) {
	username = os.Getenv("SFTP_USERNAME")
	password = os.Getenv("SFTP_PASSWORD")
	if username == "" || password == "" {
		return "", "", fmt.Errorf("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}
	return username, password, nil
}

// =============================================================================
// File Properties
// =============================================================================

// FileProperties represents the properties of an SFTP file resource.
type FileProperties struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions"`
	Size        int64  `json:"size,omitempty"`
	ModifiedAt  string `json:"modifiedAt,omitempty"`
}

// parseFileProperties extracts file properties from a JSON request.
func parseFileProperties(data json.RawMessage) (*FileProperties, error) {
	var props FileProperties
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, fmt.Errorf("invalid file properties: %w", err)
	}
	if props.Path == "" {
		return nil, fmt.Errorf("file properties missing 'path'")
	}
	if props.Permissions == "" {
		props.Permissions = "0644" // Default permissions
	}
	return &props, nil
}

// =============================================================================
// Plugin
// =============================================================================

// Plugin implements the Formae ResourcePlugin interface.
// The SDK automatically provides identity methods (Name, Version, Namespace)
// by reading formae-plugin.pkl at startup.
type Plugin struct {
	mu     sync.Mutex
	client *asyncsftp.Client
}

// Compile-time check: Plugin must satisfy ResourcePlugin interface.
var _ plugin.ResourcePlugin = &Plugin{}

// getClient returns the SFTP client, creating it if necessary.
// The client is created lazily on first use and reused for subsequent calls.
func (p *Plugin) getClient(targetConfig json.RawMessage) (*asyncsftp.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	// Parse target config
	cfg, err := parseTargetConfig(targetConfig)
	if err != nil {
		return nil, err
	}

	// Parse URL to get host and port
	host, port, err := parseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	// Get credentials from environment
	username, password, err := getCredentials()
	if err != nil {
		return nil, err
	}

	// Create client
	client, err := asyncsftp.NewClient(asyncsftp.Config{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}

	p.client = client
	return p.client, nil
}

// =============================================================================
// Configuration Methods
// =============================================================================

// RateLimit returns the rate limiting configuration for this plugin.
// SFTP servers typically limit concurrent connections, so we use a
// conservative limit of 5 requests per second.
func (p *Plugin) RateLimit() plugin.RateLimitConfig {
	return plugin.RateLimitConfig{
		Scope:                            plugin.RateLimitScopeNamespace,
		MaxRequestsPerSecondForNamespace: 5,
	}
}

// DiscoveryFilters returns filters to exclude resources from discovery.
// We discover all files, so return nil.
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter {
	return nil
}

// LabelConfig returns the configuration for extracting human-readable labels
// from discovered resources.
func (p *Plugin) LabelConfig() plugin.LabelConfig {
	return plugin.LabelConfig{
		// Use the file path as the label for discovered files
		DefaultQuery: "$.path",
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create provisions a new resource.
// Returns InProgress with a RequestID - poll Status() for completion.
func (p *Plugin) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	// Parse file properties from request
	props, err := parseFileProperties(req.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Get SFTP client
	client, err := p.getClient(req.TargetConfig)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCreate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Parse permissions string to os.FileMode
	var perm os.FileMode = 0644
	if props.Permissions != "" {
		_, _ = fmt.Sscanf(props.Permissions, "%o", &perm)
	}

	// Start async upload - returns immediately with operation ID
	requestID := client.StartUpload(props.Path, props.Content, perm)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusInProgress,
			RequestID:       requestID,
			NativeID:        props.Path, // File path is the native identifier
		},
	}, nil
}

// Read retrieves the current state of a resource.
// Returns NotFound error code (not an error) if the file doesn't exist.
func (p *Plugin) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	// Get SFTP client
	client, err := p.getClient(req.TargetConfig)
	if err != nil {
		return &resource.ReadResult{
			ResourceType: req.ResourceType,
			ErrorCode:    resource.OperationErrorCodeInternalFailure,
		}, nil
	}

	// Read file from SFTP server
	fileInfo, err := client.ReadFile(req.NativeID)
	if err != nil {
		// NotFound is not an error - return result with ErrorCode
		if errors.Is(err, asyncsftp.ErrNotFound) {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return &resource.ReadResult{
			ResourceType: req.ResourceType,
			ErrorCode:    resource.OperationErrorCodeInternalFailure,
		}, nil
	}

	// Convert to JSON properties
	props := FileProperties{
		Path:        fileInfo.Path,
		Content:     fileInfo.Content,
		Permissions: fileInfo.Permissions,
		Size:        fileInfo.Size,
		ModifiedAt:  fileInfo.ModifiedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	propsJSON, _ := json.Marshal(props)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(propsJSON),
	}, nil
}

// Update modifies an existing resource.
// Updates are synchronous - we update content and/or permissions directly.
func (p *Plugin) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Get SFTP client
	client, err := p.getClient(req.TargetConfig)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Parse desired properties
	desiredProps, err := parseFileProperties(req.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInvalidRequest,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Parse prior properties to detect changes
	priorProps, _ := parseFileProperties(req.PriorProperties)

	// Check if content changed - need to rewrite file
	if priorProps == nil || priorProps.Content != desiredProps.Content {
		var perm os.FileMode = 0644
		if desiredProps.Permissions != "" {
			_, _ = fmt.Sscanf(desiredProps.Permissions, "%o", &perm)
		}

		// Use sync upload for update (blocking)
		opID := client.StartUpload(req.NativeID, desiredProps.Content, perm)

		// Wait for completion
		for {
			op, err := client.GetStatus(opID)
			if err != nil {
				return &resource.UpdateResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationUpdate,
						OperationStatus: resource.OperationStatusFailure,
						ErrorCode:       resource.OperationErrorCodeInternalFailure,
						StatusMessage:   err.Error(),
					},
				}, nil
			}
			if op.State == asyncsftp.StateCompleted {
				break
			}
			if op.State == asyncsftp.StateFailure {
				return &resource.UpdateResult{
					ProgressResult: &resource.ProgressResult{
						Operation:       resource.OperationUpdate,
						OperationStatus: resource.OperationStatusFailure,
						ErrorCode:       resource.OperationErrorCodeInternalFailure,
						StatusMessage:   op.Error,
					},
				}, nil
			}
		}
	} else if priorProps.Permissions != desiredProps.Permissions {
		// Only permissions changed
		var perm os.FileMode = 0644
		_, _ = fmt.Sscanf(desiredProps.Permissions, "%o", &perm)

		if err := client.SetPermissions(req.NativeID, perm); err != nil {
			return &resource.UpdateResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationUpdate,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       resource.OperationErrorCodeInternalFailure,
					StatusMessage:   err.Error(),
				},
			}, nil
		}
	}

	// Read back the updated file to return current state
	fileInfo, err := client.ReadFile(req.NativeID)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationUpdate,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	resourceProps, _ := json.Marshal(FileProperties{
		Path:        fileInfo.Path,
		Content:     fileInfo.Content,
		Permissions: fileInfo.Permissions,
		Size:        fileInfo.Size,
		ModifiedAt:  fileInfo.ModifiedAt.Format("2006-01-02T15:04:05Z07:00"),
	})

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           req.NativeID,
			ResourceProperties: resourceProps,
		},
	}, nil
}

// Delete removes a resource.
// Returns Failure with NotFound error code if file doesn't exist (agent treats this as success).
func (p *Plugin) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	// Get SFTP client
	client, err := p.getClient(req.TargetConfig)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Check if file exists first
	_, err = client.ReadFile(req.NativeID)
	if err != nil {
		if errors.Is(err, asyncsftp.ErrNotFound) {
			// File doesn't exist - return Failure with NotFound
			// The agent treats NotFound on Delete as success
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       resource.OperationErrorCodeNotFound,
					NativeID:        req.NativeID,
				},
			}, nil
		}
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Start delete operation
	opID := client.StartDelete(req.NativeID)

	// Wait for completion (delete is fast, we wait synchronously)
	for {
		op, err := client.GetStatus(opID)
		if err != nil {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       resource.OperationErrorCodeInternalFailure,
					StatusMessage:   err.Error(),
				},
			}, nil
		}
		if op.State == asyncsftp.StateCompleted {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
					NativeID:        req.NativeID,
				},
			}, nil
		}
		if op.State == asyncsftp.StateFailure {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusFailure,
					ErrorCode:       resource.OperationErrorCodeInternalFailure,
					StatusMessage:   op.Error,
				},
			}, nil
		}
	}
}

// Status checks the progress of an async operation.
// Called when Create/Update/Delete return InProgress status.
func (p *Plugin) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	// Client must exist if we have a RequestID from a previous operation
	if p.client == nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   "no client available",
			},
		}, nil
	}

	// Get operation status from asyncsftp
	op, err := p.client.GetStatus(req.RequestID)
	if err != nil {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusFailure,
				ErrorCode:       resource.OperationErrorCodeInternalFailure,
				StatusMessage:   err.Error(),
			},
		}, nil
	}

	// Map asyncsftp state to resource.OperationStatus
	var status resource.OperationStatus
	var errorCode resource.OperationErrorCode
	var resourceProps json.RawMessage

	switch op.State {
	case asyncsftp.StateInProgress:
		status = resource.OperationStatusInProgress
	case asyncsftp.StateCompleted:
		status = resource.OperationStatusSuccess
		// Include resource properties on success
		if op.Result != nil {
			resourceProps, _ = json.Marshal(FileProperties{
				Path:        op.Result.Path,
				Content:     op.Result.Content,
				Permissions: op.Result.Permissions,
				Size:        op.Result.Size,
				ModifiedAt:  op.Result.ModifiedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	case asyncsftp.StateFailure:
		status = resource.OperationStatusFailure
		errorCode = resource.OperationErrorCodeInternalFailure
	}

	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCheckStatus,
			OperationStatus:    status,
			RequestID:          req.RequestID,
			NativeID:           op.Path,
			ResourceProperties: resourceProps,
			ErrorCode:          errorCode,
			StatusMessage:      op.Error,
		},
	}, nil
}

// List returns all resource identifiers of a given type.
// Called during discovery to find unmanaged resources.
func (p *Plugin) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	// Get SFTP client
	client, err := p.getClient(req.TargetConfig)
	if err != nil {
		return &resource.ListResult{
			NativeIDs: []string{},
		}, nil
	}

	// List files in the upload directory
	// In a real plugin, you might use AdditionalProperties to specify the directory
	dir := "/upload"
	if d, ok := req.AdditionalProperties["directory"]; ok {
		dir = d
	}

	paths, err := client.ListFiles(dir)
	if err != nil {
		// If directory doesn't exist, return empty list
		if errors.Is(err, asyncsftp.ErrNotFound) {
			return &resource.ListResult{
				NativeIDs: []string{},
			}, nil
		}
		return &resource.ListResult{
			NativeIDs: []string{},
		}, nil
	}

	return &resource.ListResult{
		NativeIDs:     paths,
		NextPageToken: nil, // No pagination for this simple implementation
	}, nil
}
