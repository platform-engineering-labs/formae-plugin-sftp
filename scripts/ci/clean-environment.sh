#!/bin/bash
# © 2025 Platform Engineering Labs Inc.
# SPDX-License-Identifier: FSL-1.1-ALv2
#
# Clean Environment Hook
# ======================
# This script is called before AND after conformance tests to clean up
# test resources in your cloud environment.
#
# Purpose:
# - Before tests: Remove orphaned resources from previous failed runs
# - After tests: Clean up resources created during the test run
#
# The script should be idempotent - safe to run multiple times.
# It should delete all resources matching the test resource prefix.

set -euo pipefail

# Prefix used for test resources - should match what conformance tests create
TEST_PREFIX="${TEST_PREFIX:-formae-plugin-sdk-test-}"

# SFTP connection details from environment
SFTP_HOST="${SFTP_HOST:-localhost}"
SFTP_PORT="${SFTP_PORT:-2222}"
SFTP_USERNAME="${SFTP_USERNAME:-}"
SFTP_PASSWORD="${SFTP_PASSWORD:-}"
SFTP_DIRECTORY="${SFTP_DIRECTORY:-/upload}"

echo "clean-environment.sh: Cleaning SFTP files with prefix '${TEST_PREFIX}'"

# Check for required credentials
if [[ -z "$SFTP_USERNAME" ]] || [[ -z "$SFTP_PASSWORD" ]]; then
    echo "Warning: SFTP_USERNAME and/or SFTP_PASSWORD not set, skipping cleanup"
    exit 0
fi

# Use sshpass + sftp to connect and list/delete files
# This requires sshpass to be installed
if ! command -v sshpass &> /dev/null; then
    echo "Warning: sshpass not found, skipping cleanup"
    echo "Install with: apt-get install sshpass (Debian/Ubuntu) or brew install hudochenkov/sshpass/sshpass (macOS)"
    exit 0
fi

# Create a batch file for sftp commands
BATCH_FILE=$(mktemp)
trap "rm -f $BATCH_FILE" EXIT

# List files and generate delete commands
echo "cd ${SFTP_DIRECTORY}" > "$BATCH_FILE"
echo "ls -1" >> "$BATCH_FILE"

# Get list of files
FILES=$(sshpass -p "$SFTP_PASSWORD" sftp -P "$SFTP_PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -b "$BATCH_FILE" "${SFTP_USERNAME}@${SFTP_HOST}" 2>/dev/null | grep "^${TEST_PREFIX}" || true)

if [[ -z "$FILES" ]]; then
    echo "No test files found with prefix '${TEST_PREFIX}'"
    exit 0
fi

# Create delete commands
echo "cd ${SFTP_DIRECTORY}" > "$BATCH_FILE"
while IFS= read -r file; do
    echo "rm \"$file\"" >> "$BATCH_FILE"
    echo "  Deleting: $file"
done <<< "$FILES"

# Execute delete commands
sshpass -p "$SFTP_PASSWORD" sftp -P "$SFTP_PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -b "$BATCH_FILE" "${SFTP_USERNAME}@${SFTP_HOST}" 2>/dev/null || true

echo "clean-environment.sh: Cleanup complete"
