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
		sizeBytes   int64
		purpose     upload.Purpose
		isAdmin     bool
		wantErr     bool
		wantPrefix  string
	}{
		{
			name:        "User avatar upload",
			fileName:    "profile.png",
			contentType: "image/png",
			sizeBytes:   102400,
			purpose:     upload.PurposeAvatar,
			isAdmin:     false,
			wantErr:     false,
			wantPrefix:  "avatars/",
		},
		{
			name:        "User mission proof upload",
			fileName:    "beach_cleanup.jpeg",
			contentType: "image/jpeg",
			sizeBytes:   204800,
			purpose:     upload.PurposeMissionProof,
			isAdmin:     false,
			wantErr:     false,
			wantPrefix:  "mission_proofs/",
		},
		{
			name:        "User cannot request field evidence",
			fileName:    "coral_month1.jpg",
			contentType: "image/jpeg",
			sizeBytes:   204800,
			purpose:     upload.PurposeFieldEvidence,
			isAdmin:     false,
			wantErr:     true,
		},
		{
			name:        "Admin field evidence upload",
			fileName:    "coral_month1.jpg",
			contentType: "image/jpeg",
			sizeBytes:   204800,
			purpose:     upload.PurposeFieldEvidence,
			isAdmin:     true,
			wantErr:     false,
			wantPrefix:  "field_evidence/",
		},
		{
			name:        "Admin content asset upload",
			fileName:    "hero_banner.webp",
			contentType: "image/webp",
			sizeBytes:   500000,
			purpose:     upload.PurposeContentAsset,
			isAdmin:     true,
			wantErr:     false,
			wantPrefix:  "content_assets/",
		},
		{
			name:        "Missing file name",
			fileName:    "",
			contentType: "image/jpeg",
			sizeBytes:   1000,
			purpose:     upload.PurposeAvatar,
			isAdmin:     false,
			wantErr:     true,
		},
		{
			name:        "Unsupported content type",
			fileName:    "script.pdf",
			contentType: "application/pdf",
			sizeBytes:   1000,
			purpose:     upload.PurposeAvatar,
			isAdmin:     false,
			wantErr:     true,
		},
		{
			name:        "Payload exceeds 10MB limit",
			fileName:    "huge.jpg",
			contentType: "image/jpeg",
			sizeBytes:   15 * 1024 * 1024,
			purpose:     upload.PurposeAvatar,
			isAdmin:     false,
			wantErr:     true,
		},
		{
			name:        "Payload size zero or negative",
			fileName:    "empty.jpg",
			contentType: "image/jpeg",
			sizeBytes:   0,
			purpose:     upload.PurposeAvatar,
			isAdmin:     false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := svc.RequestPresignedUpload(ctx, tt.fileName, tt.contentType, tt.sizeBytes, tt.purpose, tt.isAdmin)
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
			if res.Method != "PUT" {
				t.Errorf("expected PUT method, got %s", res.Method)
			}
			if res.Headers["Content-Type"] != tt.contentType {
				t.Errorf("expected Content-Type %s, got %s", tt.contentType, res.Headers["Content-Type"])
			}
			if res.MaxSizeBytes != upload.MaxUploadSizeBytes {
				t.Errorf("expected max_size_bytes %d, got %d", upload.MaxUploadSizeBytes, res.MaxSizeBytes)
			}
		})
	}
}
