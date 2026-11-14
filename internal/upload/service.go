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
)

// PresignResult holds the generated upload URL and S3 object key.
type PresignResult struct {
	UploadURL        string
	ObjectKey        string
	ExpiresInSeconds int
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
func (s *Service) RequestPresignedUpload(ctx context.Context, fileName, contentType string, purpose Purpose) (*PresignResult, error) {
	if strings.TrimSpace(fileName) == "" {
		return nil, errors.New("file_name is required")
	}

	var folder string
	switch purpose {
	case PurposeAvatar:
		folder = "avatars"
	case PurposeMissionProof:
		folder = "mission_proofs"
	case PurposeFieldEvidence:
		folder = "field_evidence"
	default:
		return nil, fmt.Errorf("unsupported upload purpose: %s", purpose)
	}

	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = ".jpg"
	}

	objectKey := fmt.Sprintf("%s/%s%s", folder, uuid.New().String(), ext)
	expiresIn := 900 // 15 minutes
	uploadURL := fmt.Sprintf("%s/uploads/%s?expires=%d&token=%s", s.baseURL, objectKey, time.Now().Add(15*time.Minute).Unix(), uuid.New().String()[:8])

	return &PresignResult{
		UploadURL:        uploadURL,
		ObjectKey:        objectKey,
		ExpiresInSeconds: expiresIn,
	}, nil
}
