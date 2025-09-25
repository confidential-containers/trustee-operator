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
