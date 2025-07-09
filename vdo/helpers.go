package vdo

import (
	"context"
	"strings"

	"github.com/appliedres/cloudy/logging"
)

// stringPtrOrEmpty returns the value of the string pointer or an empty string if nil.
func stringPtrOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// firstOrEmpty returns the value of the first string pointer in the slice or an empty string if the slice is empty or the first element is nil.
func firstOrEmpty(ptrs []*string) string {
	if len(ptrs) == 0 || ptrs[0] == nil {
		return ""
	}
	return *ptrs[0]
}

// stripADUsername removes the AD domain prefix or suffix from a full username.
func stripADUsername(ctx context.Context, fullUsername string) string {
	log := logging.GetLogger(ctx)
	log.DebugContext(ctx, "stripADUsername started", "fullUsername", fullUsername)

	var stripped = fullUsername

	// Strip prefix: DOMAIN\username or DOMAIN/username
	if idx := strings.IndexAny(stripped, "\\/"); idx != -1 {
		stripped = stripped[idx+1:]
	}

	// Strip suffix: username@domain.com
	if atIdx := strings.Index(stripped, "@"); atIdx != -1 {
		stripped = stripped[:atIdx]
	}

	// Final validation: no empty string, no backslash, slash, or @
	if strings.TrimSpace(stripped) == "" || strings.ContainsAny(stripped, "\\/@") {
		log.WarnContext(ctx, "Invalid AD username format after stripping. using it anyways...", "input", fullUsername, "stripped", stripped)
		return stripped
	}

	log.DebugContext(ctx, "stripADUsername complete", "stripped", stripped)
	return stripped
}
