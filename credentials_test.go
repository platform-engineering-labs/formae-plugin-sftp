// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package main

import (
	"testing"
)

func clearAmbientCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("SFTP_USERNAME", "")
	t.Setenv("SFTP_PASSWORD", "")
}

func TestCredentialsComeFromTheTargetConfigWhenDeclared(t *testing.T) {
	clearAmbientCredentials(t)
	t.Setenv("SFTP_USERNAME", "env-user")
	t.Setenv("SFTP_PASSWORD", "env-password")

	cfg, err := parseTargetConfig([]byte(`{"url":"sftp://localhost:2222","Username":"config-user","Password":"config-password"}`))
	if err != nil {
		t.Fatalf("parseTargetConfig: %v", err)
	}

	username, password, err := getCredentials(cfg)
	if err != nil {
		t.Fatalf("getCredentials: %v", err)
	}
	if username != "config-user" || password != "config-password" {
		t.Errorf("credentials = %q/%q, want the target config values", username, password)
	}
}

func TestCredentialsFallBackToTheEnvironmentWhenNotDeclared(t *testing.T) {
	clearAmbientCredentials(t)
	t.Setenv("SFTP_USERNAME", "env-user")
	t.Setenv("SFTP_PASSWORD", "env-password")

	cfg, err := parseTargetConfig([]byte(`{"url":"sftp://localhost:2222"}`))
	if err != nil {
		t.Fatalf("parseTargetConfig: %v", err)
	}

	username, password, err := getCredentials(cfg)
	if err != nil {
		t.Fatalf("getCredentials: %v", err)
	}
	if username != "env-user" || password != "env-password" {
		t.Errorf("credentials = %q/%q, want the environment values", username, password)
	}
}

// Each credential falls back on its own, so a literal username can sit beside a
// password sourced from a secret.
func TestEachCredentialFallsBackIndependently(t *testing.T) {
	clearAmbientCredentials(t)
	t.Setenv("SFTP_USERNAME", "env-user")
	t.Setenv("SFTP_PASSWORD", "env-password")

	cfg, err := parseTargetConfig([]byte(`{"url":"sftp://localhost:2222","Password":"config-password"}`))
	if err != nil {
		t.Fatalf("parseTargetConfig: %v", err)
	}

	username, password, err := getCredentials(cfg)
	if err != nil {
		t.Fatalf("getCredentials: %v", err)
	}
	if username != "env-user" {
		t.Errorf("username = %q, want the environment value", username)
	}
	if password != "config-password" {
		t.Errorf("password = %q, want the target config value", password)
	}
}

// A declared credential that resolves to nothing is a misconfiguration, not an
// invitation to log in as whoever the environment names.
func TestDeclaredButEmptyCredentialIsRejected(t *testing.T) {
	clearAmbientCredentials(t)
	t.Setenv("SFTP_USERNAME", "env-user")
	t.Setenv("SFTP_PASSWORD", "env-password")

	cfg, err := parseTargetConfig([]byte(`{"url":"sftp://localhost:2222","Password":""}`))
	if err != nil {
		t.Fatalf("parseTargetConfig: %v", err)
	}

	if _, _, err := getCredentials(cfg); err == nil {
		t.Error("getCredentials accepted a declared but empty password")
	}
}

func TestMissingCredentialsFromEverySourceIsRejected(t *testing.T) {
	clearAmbientCredentials(t)

	cfg, err := parseTargetConfig([]byte(`{"url":"sftp://localhost:2222"}`))
	if err != nil {
		t.Fatalf("parseTargetConfig: %v", err)
	}

	if _, _, err := getCredentials(cfg); err == nil {
		t.Error("getCredentials accepted a config with no credentials from any source")
	}
}
