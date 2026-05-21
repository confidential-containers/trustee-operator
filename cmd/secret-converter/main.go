/*
Copyright Confidential Containers Contributors.

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

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	repoDir    = "/opt/confidential-containers/storage/repository"
	defaultDir = "/opt/confidential-containers/storage/repository/default"
)

func main() {
	log.Println("Converting secret directories to flat files...")

	// Check if default directory exists
	if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
		log.Printf("No secrets found in %s, skipping conversion", defaultDir)
		return
	} else if err != nil {
		log.Fatalf("Error checking default directory: %v", err)
	}

	// Check if directory is empty
	entries, err := os.ReadDir(defaultDir)
	if err != nil {
		log.Fatalf("Error reading default directory: %v", err)
	}
	if len(entries) == 0 {
		log.Printf("No secrets found in %s, skipping conversion", defaultDir)
		return
	}

	// Process each secret directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		secretName := entry.Name()
		secretPath := filepath.Join(defaultDir, secretName)
		log.Printf("Processing secret: %s", secretName)

		if err := processSecretDir(secretName, secretPath); err != nil {
			log.Fatalf("Error processing secret %s: %v", secretName, err)
		}
	}

	log.Println("Secret conversion complete")
	if err := listConvertedFiles(); err != nil {
		log.Printf("Warning: could not list converted files: %v", err)
	}
}

// processSecretDir processes all files in a secret directory and converts them to flat files
func processSecretDir(secretName, secretPath string) error {
	entries, err := os.ReadDir(secretPath)
	if err != nil {
		return fmt.Errorf("reading secret directory: %w", err)
	}

	for _, entry := range entries {
		// Skip hidden files and directories
		if strings.HasPrefix(entry.Name(), ".") || entry.IsDir() {
			continue
		}

		keyFile := entry.Name()
		sourcePath := filepath.Join(secretPath, keyFile)

		// Follow symlinks to get the real file
		realPath, err := filepath.EvalSymlinks(sourcePath)
		if err != nil {
			return fmt.Errorf("resolving symlink for %s: %w", sourcePath, err)
		}

		// Check if it's a regular file
		info, err := os.Stat(realPath)
		if err != nil {
			return fmt.Errorf("stat file %s: %w", realPath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		// Create flat file name with escaped slashes: default\x2Fsecret\x2Fkey
		flatName := fmt.Sprintf("default\\x2F%s\\x2F%s", secretName, keyFile)
		destPath := filepath.Join(repoDir, flatName)

		log.Printf("  Converting %s -> %s", keyFile, flatName)

		if err := copyFile(realPath, destPath); err != nil {
			return fmt.Errorf("copying %s to %s: %w", realPath, destPath, err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer func() {
		if cerr := sourceFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing source file: %w", cerr)
		}
	}()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer func() {
		if cerr := destFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing destination file: %w", cerr)
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("copying file content: %w", err)
	}

	return destFile.Sync()
}

// listConvertedFiles lists all converted files in the repository directory
func listConvertedFiles() error {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return err
	}

	converted := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "\\x2F") {
			converted = true
			break
		}
	}

	if !converted {
		log.Println("No converted files found (this might be an issue)")
	}

	return nil
}
