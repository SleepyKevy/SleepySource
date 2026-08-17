//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type credentialDataBlob struct {
	Size uint32
	Data *byte
}

var (
	crypt32Cred                = syscall.NewLazyDLL("crypt32.dll")
	kernel32Cred               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectDataCred   = crypt32Cred.NewProc("CryptProtectData")
	procCryptUnprotectDataCred = crypt32Cred.NewProc("CryptUnprotectData")
	procLocalFreeCred          = kernel32Cred.NewProc("LocalFree")
)

const cryptProtectUIForbidden = 0x1

func secureCredentialStorageAvailable() bool { return true }

func credentialBlob(data []byte) credentialDataBlob {
	if len(data) == 0 {
		return credentialDataBlob{}
	}
	return credentialDataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func credentialBlobBytes(blob *credentialDataBlob) []byte {
	if blob == nil || blob.Size == 0 || blob.Data == nil {
		return nil
	}
	out := append([]byte(nil), unsafe.Slice(blob.Data, int(blob.Size))...)
	procLocalFreeCred.Call(uintptr(unsafe.Pointer(blob.Data)))
	blob.Data = nil
	blob.Size = 0
	return out
}

func protectCredential(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	in := credentialBlob(plain)
	entropyBytes := []byte("SleepySource Kick Credentials v1")
	entropy := credentialBlob(entropyBytes)
	var out credentialDataBlob
	r, _, callErr := procCryptProtectDataCred.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		uintptr(unsafe.Pointer(&entropy)),
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("Windows could not encrypt the Kick Client Secret: %v", callErr)
	}
	return credentialBlobBytes(&out), nil
}

func unprotectCredential(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	in := credentialBlob(ciphertext)
	entropyBytes := []byte("SleepySource Kick Credentials v1")
	entropy := credentialBlob(entropyBytes)
	var out credentialDataBlob
	r, _, callErr := procCryptUnprotectDataCred.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		uintptr(unsafe.Pointer(&entropy)),
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("Windows could not decrypt the saved Kick Client Secret: %v", callErr)
	}
	return credentialBlobBytes(&out), nil
}
