package upload_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Drewron01/bioverse-backend/internal/upload"
)

func TestRequestPresignedUpload(t *testing.T) {
	svc := upload.NewService("https://storage.bioverse.id")
	ctx := context.Background()

	tests := []struct {
		name        string
		fileName    string
		contentType string
		purpose     upload.Purpose
		wantErr     bool
		wantPrefix  string
	}{
		{
			name:        "Avatar upload",
			fileName:    "profile.png",
			contentType: "image/png",
			purpose:     upload.PurposeAvatar,
			wantErr:     false,
			wantPrefix:  "avatars/",
		},
		{
			name:        "Mission proof upload",
			fileName:    "beach_cleanup.jpeg",
			contentType: "image/jpeg",
			purpose:     upload.PurposeMissionProof,
			wantErr:     false,
			wantPrefix:  "mission_proofs/",
		},
		{
			name:        "Field evidence upload",
			fileName:    "coral_month1.jpg",
			contentType: "image/jpeg",
			purpose:     upload.PurposeFieldEvidence,
			wantErr:     false,
			wantPrefix:  "field_evidence/",
		},
		{
			name:        "Missing file name",
			fileName:    "",
			contentType: "image/jpeg",
			purpose:     upload.PurposeAvatar,
			wantErr:     true,
		},
		{
			name:        "Unsupported purpose",
			fileName:    "doc.pdf",
			contentType: "application/pdf",
			purpose:     upload.Purpose("unsupported_purpose"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := svc.RequestPresignedUpload(ctx, tt.fileName, tt.contentType, tt.purpose)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(res.ObjectKey, tt.wantPrefix) {
				t.Errorf("expected object key to start with %s, got %s", tt.wantPrefix, res.ObjectKey)
			}
			if res.ExpiresInSeconds != 900 {
				t.Errorf("expected 900s expiration, got %d", res.ExpiresInSeconds)
			}
			if !strings.HasPrefix(res.UploadURL, "https://storage.bioverse.id/uploads/") {
				t.Errorf("unexpected upload URL: %s", res.UploadURL)
			}
		})
	}
}
