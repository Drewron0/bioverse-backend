package upload

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
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

// PresignResult holds the generated upload target URL, S3 object key, HTTP method, and required headers.
type PresignResult struct {
	UploadURL        string            `json:"upload_url"`
	ObjectKey        string            `json:"object_key"`
	ExpiresInSeconds int               `json:"expires_in_seconds"`
	Method           string            `json:"method"`
	Headers          map[string]string `json:"headers"`
	MaxSizeBytes     int64             `json:"max_size_bytes"`
}

// Config defines connection details for S3 / MinIO storage.
type Config struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	BucketName string
	Region     string
	UseSSL     bool
}

// Service provides AWS SigV4 presigned URL generation for client uploads.
type Service struct {
	cfg Config
}

// NewService constructs a new upload service with S3/MinIO configuration.
func NewService(cfg Config) *Service {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://127.0.0.1:9000"
	}
	if cfg.BucketName == "" {
		cfg.BucketName = "bioverse-media"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = "bioverse_minio"
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "bioverse_minio_secret"
	}
	return &Service{cfg: cfg}
}

// RequestPresignedUpload validates constraints and generates an authentic AWS SigV4 presigned PUT target.
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

	presignedURL, err := s.signS3PutURL(objectKey, contentType, expiresIn)
	if err != nil {
		return nil, fmt.Errorf("failed to sign presigned upload target: %w", err)
	}

	return &PresignResult{
		UploadURL:        presignedURL,
		ObjectKey:        objectKey,
		ExpiresInSeconds: expiresIn,
		Method:           "PUT",
		Headers: map[string]string{
			"Content-Type": contentType,
		},
		MaxSizeBytes: MaxUploadSizeBytes,
	}, nil
}

// signS3PutURL calculates standard AWS SigV4 presigned PUT target URL.
func (s *Service) signS3PutURL(objectKey, contentType string, expiresInSeconds int) (string, error) {
	parsedURL, err := url.Parse(s.cfg.Endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid storage endpoint URL: %w", err)
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.cfg.Region)
	credentialParam := fmt.Sprintf("%s/%s", s.cfg.AccessKey, credentialScope)

	canonicalURI := fmt.Sprintf("/%s/%s", s.cfg.BucketName, strings.TrimPrefix(objectKey, "/"))

	host := parsedURL.Host
	if host == "" {
		host = "127.0.0.1:9000"
	}

	signedHeaders := "content-type;host"
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\n", strings.ToLower(strings.TrimSpace(contentType)), host)

	queryParams := url.Values{}
	queryParams.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	queryParams.Set("X-Amz-Credential", credentialParam)
	queryParams.Set("X-Amz-Date", amzDate)
	queryParams.Set("X-Amz-Expires", fmt.Sprintf("%d", expiresInSeconds))
	queryParams.Set("X-Amz-SignedHeaders", signedHeaders)

	canonicalQueryString := queryParams.Encode()

	canonicalRequest := strings.Join([]string{
		"PUT",
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	reqHash := sha256Hex([]byte(canonicalRequest))

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		reqHash,
	}, "\n")

	signingKey := getSignatureKey(s.cfg.SecretKey, dateStamp, s.cfg.Region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	finalURL := fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s",
		strings.TrimRight(s.cfg.Endpoint, "/"),
		canonicalURI,
		canonicalQueryString,
		signature,
	)

	return finalURL, nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(secretKey, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	return hmacSHA256(kService, []byte("aws4_request"))
}
