//go:build !windows

package main

import "fmt"

func secureCredentialStorageAvailable() bool { return false }
func protectCredential([]byte) ([]byte, error) {
	return nil, fmt.Errorf("secure credential storage is only available on Windows")
}
func unprotectCredential([]byte) ([]byte, error) {
	return nil, fmt.Errorf("secure credential storage is only available on Windows")
}
