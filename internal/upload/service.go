package upload

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Purpose represents allowed presigned upload categories.
type Purpose string

const (
	PurposeAvatar        Purpose = "avatar"
	PurposeMissionProof  Purpose = "mission_proof"
	PurposeFieldEvidence Purpose = "field_evidence"
	PurposeContentAsset  Purpose = "content_asset"
)

// MaxUploadSizeBytes defines the maximum allowed upload payload (10 MB).
const MaxUploadSizeBytes int64 = 10485760

// PresignResult holds the generated upload URL, S3 object key, HTTP method, and required headers.
type PresignResult struct {
	UploadURL        string            `json:"upload_url"`
	ObjectKey        string            `json:"object_key"`
	ExpiresInSeconds int               `json:"expires_in_seconds"`
	Method           string            `json:"method"`
	Headers          map[string]string `json:"headers"`
	MaxSizeBytes     int64             `json:"max_size_bytes"`
}

// Service provides presigned URL generation for client uploads.
type Service struct {
	baseURL string
}

// NewService constructs a new upload service.
func NewService(baseURL string) *Service {
	if baseURL == "" {
		baseURL = "https://storage.bioverse.id"
	}
	return &Service{baseURL: strings.TrimRight(baseURL, "/")}
}

// RequestPresignedUpload generates an authenticated upload target.
func (s *Service) RequestPresignedUpload(ctx context.Context, fileName, contentType string, sizeBytes int64, purpose Purpose, isAdmin bool) (*PresignResult, error) {
	if strings.TrimSpace(fileName) == "" {
		return nil, errors.New("file_name is required")
	}

	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}
	if !validTypes[contentType] {
		return nil, fmt.Errorf("unsupported content_type '%s', allowed: image/jpeg, image/png, image/webp", contentType)
	}

	if sizeBytes <= 0 || sizeBytes > MaxUploadSizeBytes {
		return nil, fmt.Errorf("size_bytes must be between 1 and %d (10 MB)", MaxUploadSizeBytes)
	}

	var folder string
	if isAdmin {
		switch purpose {
		case PurposeFieldEvidence:
			folder = "field_evidence"
		case PurposeContentAsset:
			folder = "content_assets"
		default:
			return nil, fmt.Errorf("admin upload purpose must be 'field_evidence' or 'content_asset', got '%s'", purpose)
		}
	} else {
		switch purpose {
		case PurposeAvatar:
			folder = "avatars"
		case PurposeMissionProof:
			folder = "mission_proofs"
		default:
			return nil, fmt.Errorf("user upload purpose must be 'avatar' or 'mission_proof', got '%s'", purpose)
		}
	}

	ext := filepath.Ext(fileName)
	if ext == "" {
		switch contentType {
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		default:
			ext = ".jpg"
		}
	}

	objectKey := fmt.Sprintf("%s/%s%s", folder, uuid.New().String(), ext)
	expiresIn := 900 // 15 minutes
	uploadURL := fmt.Sprintf("%s/uploads/%s?expires=%d&token=%s", s.baseURL, objectKey, time.Now().Add(15*time.Minute).Unix(), uuid.New().String()[:8])

	return &PresignResult{
		UploadURL:        uploadURL,
		ObjectKey:        objectKey,
		ExpiresInSeconds: expiresIn,
		Method:           "PUT",
		Headers: map[string]string{
			"Content-Type": contentType,
		},
		MaxSizeBytes: MaxUploadSizeBytes,
	}, nil
}
