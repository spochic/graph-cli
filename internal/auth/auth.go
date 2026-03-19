/*
Copyright © 2025 Sebastien Pochic <spochic@gmail.com>
*/
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
)

const recordFileName = "auth_record.json"

// RecordPath returns the path where the AuthenticationRecord is stored.
func RecordPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config directory: %w", err)
	}
	return filepath.Join(dir, "graph-cli", recordFileName), nil
}

// SaveRecord serializes an AuthenticationRecord to disk.
func SaveRecord(record azidentity.AuthenticationRecord) error {
	path, err := RecordPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}
	b, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("could not serialize auth record: %w", err)
	}
	return os.WriteFile(path, b, 0600)
}

// LoadRecord deserializes a previously saved AuthenticationRecord from disk.
// The second return value is false if no record exists yet.
func LoadRecord() (azidentity.AuthenticationRecord, bool, error) {
	path, err := RecordPath()
	if err != nil {
		return azidentity.AuthenticationRecord{}, false, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return azidentity.AuthenticationRecord{}, false, nil
	}
	if err != nil {
		return azidentity.AuthenticationRecord{}, false, fmt.Errorf("could not read auth record: %w", err)
	}
	var record azidentity.AuthenticationRecord
	if err := json.Unmarshal(b, &record); err != nil {
		return azidentity.AuthenticationRecord{}, false, fmt.Errorf("could not parse auth record: %w", err)
	}
	return record, true, nil
}

// DeleteRecord removes the saved AuthenticationRecord from disk.
// Returns nil if no record exists.
func DeleteRecord() error {
	path, err := RecordPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// NewTokenCache creates a persistent token cache.
func NewTokenCache() (azidentity.Cache, error) {
	c, err := cache.New(&cache.Options{Name: "graph-cli"})
	if err != nil {
		return azidentity.Cache{}, fmt.Errorf("failed to initialize token cache: %w", err)
	}
	return c, nil
}
