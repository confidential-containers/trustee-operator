/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// encodeEd25519PrivateKeyToPEM encodes an Ed25519 private key to PEM format
func encodeEd25519PrivateKeyToPEM(privateKey ed25519.PrivateKey) ([]byte, error) {
	// Encode private key to PKCS#8 format
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	// Create PEM block
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return privateKeyPEM, nil
}

// encodeEd25519PublicKeyToPEM encodes an Ed25519 public key to PEM format
func encodeEd25519PublicKeyToPEM(publicKey ed25519.PublicKey) ([]byte, error) {
	// Encode public key to PKIX format
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	// Create PEM block
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return publicKeyPEM, nil
}

// Legacy functions for backward compatibility (keeping the old function names)
// These now delegate to the Ed25519 functions

// encodePrivateKeyToPEM encodes an RSA private key to PEM format
// Deprecated: Use encodeEd25519PrivateKeyToPEM for Ed25519 keys
func encodePrivateKeyToPEM(privateKey interface{}) ([]byte, error) {
	if ed25519Key, ok := privateKey.(ed25519.PrivateKey); ok {
		return encodeEd25519PrivateKeyToPEM(ed25519Key)
	}
	// Fallback for other key types if needed
	return nil, fmt.Errorf("unsupported private key type")
}

// encodePublicKeyToPEM encodes an RSA public key to PEM format
// Deprecated: Use encodeEd25519PublicKeyToPEM for Ed25519 keys
func encodePublicKeyToPEM(publicKey interface{}) ([]byte, error) {
	if ed25519Key, ok := publicKey.(ed25519.PublicKey); ok {
		return encodeEd25519PublicKeyToPEM(ed25519Key)
	}
	// Fallback for other key types if needed
	return nil, fmt.Errorf("unsupported public key type")
}
