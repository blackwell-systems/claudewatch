package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseAllFileHistory reads ~/.claude/file-history/ and returns per-session
// edit metadata. It does not read file contents — only collects counts and sizes.
func ParseAllFileHistory(claudeHome string) ([]FileHistorySession, error) {
	dir := filepath.Join(claudeHome, "file-history")
	sessionDirs, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []FileHistorySession
	for _, sd := range sessionDirs {
		if !sd.IsDir() {
			continue
		}

		sessionDir := filepath.Join(dir, sd.Name())
		files, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}

		hashes := make(map[string]bool)
		var totalEdits int
		var maxVersion int
		var totalBytes int64

		for _, f := range files {
			if f.IsDir() {
				continue
			}

			hash, version := parseVersionedFilename(f.Name())
			if hash == "" {
				continue
			}

			hashes[hash] = true
			totalEdits++
			if version > maxVersion {
				maxVersion = version
			}

			if info, err := f.Info(); err == nil {
				totalBytes += info.Size()
			}
		}

		if len(hashes) == 0 {
			continue
		}

		results = append(results, FileHistorySession{
			SessionID:   sd.Name(),
			UniqueFiles: len(hashes),
			TotalEdits:  totalEdits,
			MaxVersion:  maxVersion,
			TotalBytes:  totalBytes,
		})
	}
	return results, nil
}

// parseVersionedFilename splits "hash@vN" into the hash and version number.
func parseVersionedFilename(name string) (hash string, version int) {
	idx := strings.LastIndex(name, "@v")
	if idx < 0 {
		return "", 0
	}
	hash = name[:idx]
	v, err := strconv.Atoi(name[idx+2:])
	if err != nil {
		return "", 0
	}
	return hash, v
}
