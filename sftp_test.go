// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-sftp/pkg/asyncsftp"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test Configuration
// =============================================================================

// testConfig returns the SFTP connection configuration for tests.
// Requires environment variables: SFTP_USERNAME, SFTP_PASSWORD
// Requires Docker container: docker run -p 2222:22 -d --name sftp-test atmoz/sftp testuser:testpass:::upload
func testConfig() asyncsftp.Config {
	return asyncsftp.Config{
		Host:     "localhost",
		Port:     "2222",
		Username: os.Getenv("SFTP_USERNAME"),
		Password: os.Getenv("SFTP_PASSWORD"),
	}
}

// testTargetConfig returns the target configuration JSON for plugin requests.
func testTargetConfig() json.RawMessage {
	return json.RawMessage(`{"url": "sftp://localhost:2222"}`)
}

// =============================================================================
// Create Tests
// =============================================================================

// TestCreate verifies that the Plugin.Create method:
// 1. Returns InProgress status with a RequestID (async pattern)
// 2. Eventually completes successfully when polled via Status
// 3. Actually creates the file on the SFTP server
//
// The async pattern works as follows:
// - Create() starts the operation and returns immediately with InProgress
// - The agent polls Status() with the RequestID until Success or Failure
// - On Success, the file exists on the server
func TestCreate(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()
	plugin := &Plugin{}

	// Test file properties
	filePath := "/upload/test-create.txt"
	fileContent := "Hello from Create test"
	filePermissions := "0644"

	// Build the CreateRequest
	properties := map[string]any{
		"path":        filePath,
		"content":     fileContent,
		"permissions": filePermissions,
	}
	propertiesJSON, err := json.Marshal(properties)
	require.NoError(t, err, "failed to marshal properties")

	req := &resource.CreateRequest{
		ResourceType: "SFTP::Files::File",
		Label:        "test-create",
		Properties:   propertiesJSON,
		TargetConfig: testTargetConfig(),
	}

	// --- Step 1: Call Create and verify InProgress + RequestID ---
	result, err := plugin.Create(ctx, req)
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, result.ProgressResult, "Create should return ProgressResult")

	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus,
		"Create should return InProgress status")
	assert.NotEmpty(t, result.ProgressResult.RequestID,
		"Create should return RequestID for async operation")

	requestID := result.ProgressResult.RequestID
	t.Logf("Create returned RequestID: %s", requestID)

	// --- Step 2: Poll Status until Success ---
	statusReq := &resource.StatusRequest{
		RequestID:    requestID,
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	require.Eventually(t, func() bool {
		statusResult, err := plugin.Status(ctx, statusReq)
		if err != nil {
			t.Logf("Status error: %v", err)
			return false
		}
		if statusResult.ProgressResult == nil {
			return false
		}
		status := statusResult.ProgressResult.OperationStatus
		t.Logf("Status poll: %s", status)

		if status == resource.OperationStatusFailure {
			t.Errorf("operation failed: %s", statusResult.ProgressResult.StatusMessage)
			return true // Stop polling
		}
		return status == resource.OperationStatusSuccess
	}, 10*time.Second, 100*time.Millisecond, "Create operation should complete successfully")

	// --- Step 3: Verify file exists using asyncsftp ---
	client, err := asyncsftp.NewClient(testConfig())
	require.NoError(t, err, "failed to create asyncsftp client")
	defer client.Close()

	fileInfo, err := client.ReadFile(filePath)
	require.NoError(t, err, "file should exist after Create")

	assert.Equal(t, fileContent, fileInfo.Content, "content should match")
	assert.Equal(t, filePermissions, fileInfo.Permissions, "permissions should match")

	t.Logf("File created successfully: path=%s, size=%d", fileInfo.Path, fileInfo.Size)

	// --- Cleanup ---
	_ = client.StartDelete(filePath)
}

// =============================================================================
// Read Tests
// =============================================================================

// TestRead verifies that the Plugin.Read method:
// 1. Returns the current state of an existing file
// 2. Properties match what was written to the server
//
// Setup: Create file using asyncsftp
// Execute: Read using Plugin
// Verify: Assert on the returned properties
func TestRead(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()

	// --- Setup: Create file using asyncsftp ---
	client, err := asyncsftp.NewClient(testConfig())
	require.NoError(t, err, "failed to create asyncsftp client")
	defer client.Close()

	filePath := "/upload/test-read.txt"
	fileContent := "Hello from Read test"
	filePermissions := os.FileMode(0644)

	opID := client.StartUpload(filePath, fileContent, filePermissions)

	// Wait for upload to complete
	require.Eventually(t, func() bool {
		op, err := client.GetStatus(opID)
		if err != nil {
			return false
		}
		return op.State == asyncsftp.StateCompleted
	}, 10*time.Second, 100*time.Millisecond, "file upload should complete")

	// --- Execute: Read using Plugin ---
	plugin := &Plugin{}

	req := &resource.ReadRequest{
		NativeID:     filePath,
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	result, err := plugin.Read(ctx, req)
	require.NoError(t, err, "Read should not return error")

	// ErrorCode should be empty for existing file
	assert.Empty(t, result.ErrorCode, "Read should not return error code for existing file")

	// --- Verify: Assert on returned properties ---
	require.NotEmpty(t, result.Properties, "Read should return properties")

	var props map[string]any
	err = json.Unmarshal([]byte(result.Properties), &props)
	require.NoError(t, err, "Properties should be valid JSON")

	assert.Equal(t, filePath, props["path"], "path should match")
	assert.Equal(t, fileContent, props["content"], "content should match")
	assert.Equal(t, "0644", props["permissions"], "permissions should match")
	assert.NotEmpty(t, props["size"], "size should be set")
	assert.NotEmpty(t, props["modifiedAt"], "modifiedAt should be set")

	t.Logf("Read returned properties: path=%s, size=%v", props["path"], props["size"])

	// --- Cleanup ---
	_ = client.StartDelete(filePath)
}

// TestReadNotFound verifies that Plugin.Read returns OperationErrorCodeNotFound
// when the file does not exist.
//
// This is critical for formae's synchronization mechanism. The agent periodically
// calls Read on all managed resources to sync formae's state with the actual
// infrastructure state. When a resource is deleted out-of-band (outside of formae),
// the plugin must return NotFound (not an error). The agent then marks the resource
// as deleted in its inventory.
//
// Important: Plugins must return a result with ErrorCode=NotFound, NOT an error.
func TestReadNotFound(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()
	plugin := &Plugin{}

	// Read a file that does not exist
	req := &resource.ReadRequest{
		NativeID:     "/upload/this-file-does-not-exist.txt",
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	result, err := plugin.Read(ctx, req)

	// IMPORTANT: Read should NOT return an error for NotFound
	require.NoError(t, err, "Read should not return error for missing file")

	// Should return NotFound error code
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode,
		"Read should return NotFound error code for missing file")

	// Properties should be empty
	assert.Empty(t, result.Properties, "Properties should be empty for missing file")

	t.Log("Read correctly returned NotFound for non-existent file")
}

// =============================================================================
// Update Tests
// =============================================================================

// TestUpdate verifies that the Plugin.Update method:
// 1. Updates file content and/or permissions
// 2. Returns Success with updated properties
//
// Setup: Create file using asyncsftp
// Execute: Update using Plugin
// Verify: Read back using asyncsftp to confirm changes
func TestUpdate(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()

	// --- Setup: Create file using asyncsftp ---
	client, err := asyncsftp.NewClient(testConfig())
	require.NoError(t, err, "failed to create asyncsftp client")
	defer client.Close()

	filePath := "/upload/test-update.txt"
	originalContent := "Original content"
	originalPermissions := os.FileMode(0644)

	opID := client.StartUpload(filePath, originalContent, originalPermissions)

	require.Eventually(t, func() bool {
		op, _ := client.GetStatus(opID)
		return op != nil && op.State == asyncsftp.StateCompleted
	}, 10*time.Second, 100*time.Millisecond, "file upload should complete")

	// --- Execute: Update using Plugin ---
	plugin := &Plugin{}

	priorProps, _ := json.Marshal(map[string]any{
		"path":        filePath,
		"content":     originalContent,
		"permissions": "0644",
	})

	updatedContent := "Updated content"
	updatedPermissions := "0600"

	desiredProps, _ := json.Marshal(map[string]any{
		"path":        filePath,
		"content":     updatedContent,
		"permissions": updatedPermissions,
	})

	req := &resource.UpdateRequest{
		NativeID:          filePath,
		ResourceType:      "SFTP::Files::File",
		PriorProperties:   priorProps,
		DesiredProperties: desiredProps,
		TargetConfig:      testTargetConfig(),
	}

	result, err := plugin.Update(ctx, req)
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, result.ProgressResult, "Update should return ProgressResult")

	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus,
		"Update should return Success status")

	// --- Verify: Read back using asyncsftp ---
	fileInfo, err := client.ReadFile(filePath)
	require.NoError(t, err, "file should exist after Update")

	assert.Equal(t, updatedContent, fileInfo.Content, "content should be updated")
	assert.Equal(t, updatedPermissions, fileInfo.Permissions, "permissions should be updated")

	t.Logf("File updated successfully: content=%q, permissions=%s", fileInfo.Content, fileInfo.Permissions)

	// --- Cleanup ---
	_ = client.StartDelete(filePath)
}

// =============================================================================
// Delete Tests
// =============================================================================

// TestDelete verifies that the Plugin.Delete method:
// 1. Deletes the file from the server
// 2. Returns Success status
//
// Setup: Create file using asyncsftp
// Execute: Delete using Plugin
// Verify: Confirm file is gone using asyncsftp
func TestDelete(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()

	// --- Setup: Create file using asyncsftp ---
	client, err := asyncsftp.NewClient(testConfig())
	require.NoError(t, err, "failed to create asyncsftp client")
	defer client.Close()

	filePath := "/upload/test-delete.txt"
	fileContent := "File to be deleted"

	opID := client.StartUpload(filePath, fileContent, 0644)

	require.Eventually(t, func() bool {
		op, _ := client.GetStatus(opID)
		return op != nil && op.State == asyncsftp.StateCompleted
	}, 10*time.Second, 100*time.Millisecond, "file upload should complete")

	// Verify file exists
	_, err = client.ReadFile(filePath)
	require.NoError(t, err, "file should exist before Delete")

	// --- Execute: Delete using Plugin ---
	plugin := &Plugin{}

	req := &resource.DeleteRequest{
		NativeID:     filePath,
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	result, err := plugin.Delete(ctx, req)
	require.NoError(t, err, "Delete should not return error")
	require.NotNil(t, result.ProgressResult, "Delete should return ProgressResult")

	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus,
		"Delete should return Success status")

	// --- Verify: Confirm file is gone using asyncsftp ---
	_, err = client.ReadFile(filePath)
	assert.ErrorIs(t, err, asyncsftp.ErrNotFound, "file should not exist after Delete")

	t.Log("File deleted successfully")
}

// TestDeleteNotFound verifies that Plugin.Delete returns Failure with NotFound
// when the file doesn't exist.
//
// The agent treats NotFound on Delete as success - the desired state (file gone)
// is already achieved.
func TestDeleteNotFound(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()
	plugin := &Plugin{}

	// Delete a file that does not exist
	req := &resource.DeleteRequest{
		NativeID:     "/upload/this-file-does-not-exist-for-delete.txt",
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	result, err := plugin.Delete(ctx, req)

	// Delete should NOT return an error for NotFound
	require.NoError(t, err, "Delete should not return error for missing file")
	require.NotNil(t, result.ProgressResult, "Delete should return ProgressResult")

	// Should return Failure status with NotFound error code
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus,
		"Delete should return Failure status for missing file")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ProgressResult.ErrorCode,
		"Delete should return NotFound error code for missing file")

	t.Log("Delete correctly returned Failure with NotFound for non-existent file")
}

// =============================================================================
// List Tests
// =============================================================================

// TestList verifies that the Plugin.List method:
// 1. Returns all file paths in the directory
// 2. Can be used for discovery of unmanaged resources
//
// Setup: Create multiple files using asyncsftp
// Execute: List using Plugin
// Verify: Assert returned paths include our files
func TestList(t *testing.T) {
	if os.Getenv("SFTP_USERNAME") == "" || os.Getenv("SFTP_PASSWORD") == "" {
		t.Skip("SFTP_USERNAME and SFTP_PASSWORD must be set")
	}

	ctx := context.Background()

	// --- Setup: Create files using asyncsftp ---
	client, err := asyncsftp.NewClient(testConfig())
	require.NoError(t, err, "failed to create asyncsftp client")
	defer client.Close()

	testFiles := []string{
		"/upload/test-list-1.txt",
		"/upload/test-list-2.txt",
		"/upload/test-list-3.txt",
	}

	for _, filePath := range testFiles {
		opID := client.StartUpload(filePath, "content", 0644)
		require.Eventually(t, func() bool {
			op, _ := client.GetStatus(opID)
			return op != nil && op.State == asyncsftp.StateCompleted
		}, 10*time.Second, 100*time.Millisecond, "file upload should complete")
	}

	// --- Execute: List using Plugin ---
	plugin := &Plugin{}

	req := &resource.ListRequest{
		ResourceType: "SFTP::Files::File",
		TargetConfig: testTargetConfig(),
	}

	result, err := plugin.List(ctx, req)
	require.NoError(t, err, "List should not return error")

	// --- Verify: Assert returned paths include our files ---
	for _, filePath := range testFiles {
		assert.Contains(t, result.NativeIDs, filePath,
			"List should include %s", filePath)
	}

	t.Logf("List returned %d files", len(result.NativeIDs))

	// --- Cleanup ---
	for _, filePath := range testFiles {
		_ = client.StartDelete(filePath)
	}
}
