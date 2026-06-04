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

package controllers

import (
	"bufio"
	"strings"
)

// mergeTlsSettings merges TLS settings from new config into existing config
// Preserves all non-TLS settings from the existing config
func mergeTlsSettings(existingConfig, newConfig string) string {
	// Extract TLS lines from the new config
	tlsLines := extractTlsLines(newConfig)
	if len(tlsLines) == 0 {
		// No TLS settings in new config, return existing as-is
		return existingConfig
	}

	// Remove old TLS lines from existing config and add new ones
	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(existingConfig))
	inHttpServer := false
	httpServerWritten := false
	inTlsBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track if we're in [http_server] section
		if trimmed == "[http_server]" {
			inHttpServer = true
			result.WriteString(line + "\n")
			continue
		} else if strings.HasPrefix(trimmed, "[") && trimmed != "[http_server]" {
			// Entering a different section
			if inHttpServer && !httpServerWritten {
				// Write TLS settings before leaving http_server section
				for _, tlsLine := range tlsLines {
					result.WriteString(tlsLine + "\n")
				}
				httpServerWritten = true
			}
			inHttpServer = false
			inTlsBlock = false
		}

		// Track if we're in the TLS block
		if inHttpServer && isTlsLine(trimmed) {
			if !inTlsBlock {
				inTlsBlock = true
			}
			// Skip TLS line
			continue
		}

		// Skip blank lines within the TLS block
		if inHttpServer && inTlsBlock && trimmed == "" {
			continue
		}

		// If we hit a non-TLS, non-blank line, we're out of the TLS block
		if inHttpServer && inTlsBlock && trimmed != "" && !isTlsLine(trimmed) {
			inTlsBlock = false
		}

		result.WriteString(line + "\n")
	}

	// If we reached EOF while still in http_server, add TLS lines at the end
	if inHttpServer && !httpServerWritten {
		for _, tlsLine := range tlsLines {
			result.WriteString(tlsLine + "\n")
		}
	}

	return result.String()
}

// extractTlsLines extracts TLS-related lines from the http_server section
func extractTlsLines(config string) []string {
	var tlsLines []string
	scanner := bufio.NewScanner(strings.NewReader(config))
	inHttpServer := false
	inTlsBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "[http_server]" {
			inHttpServer = true
			continue
		} else if strings.HasPrefix(trimmed, "[") {
			// Exiting http_server section
			break
		}

		if inHttpServer {
			if strings.HasPrefix(trimmed, "# TLS configuration") {
				inTlsBlock = true
			}
			if inTlsBlock || isTlsLine(trimmed) {
				tlsLines = append(tlsLines, line)
			}
		}
	}

	return tlsLines
}

// isTlsLine checks if a line is a TLS configuration setting
func isTlsLine(line string) bool {
	return strings.HasPrefix(line, "tls_profile =") ||
		strings.HasPrefix(line, "tls_min_version =") ||
		strings.HasPrefix(line, "tls_max_version =") ||
		strings.HasPrefix(line, "tls_ciphers =") ||
		strings.HasPrefix(line, "tls_groups =") ||
		strings.HasPrefix(line, "# TLS configuration")
}
