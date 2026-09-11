package service

import (
	"context"
	"encoding/json"
	"time"
)

// RequestRecord is the complete wire-level gateway request/response pair.
// Bodies use []byte so binary endpoints (audio, images, etc.) are preserved;
// encoding/json exposes them as base64 when returned by the admin API.
type RequestRecord struct {
	ID                  int64                   `json:"id"`
	RequestID           string                  `json:"request_id,omitempty"`
	ClientRequestID     string                  `json:"client_request_id,omitempty"`
	UserID              *int64                  `json:"user_id,omitempty"`
	APIKeyID            *int64                  `json:"api_key_id,omitempty"`
	AccountID           *int64                  `json:"account_id,omitempty"`
	GroupID             *int64                  `json:"group_id,omitempty"`
	Model               string                  `json:"model,omitempty"`
	Method              string                  `json:"method"`
	Path                string                  `json:"path"`
	QueryString         string                  `json:"query_string,omitempty"`
	RequestHeaders      json.RawMessage         `json:"request_headers"`
	RequestBody         []byte                  `json:"request_body,omitempty"`
	RequestContentType  string                  `json:"request_content_type,omitempty"`
	ResponseStatus      int                     `json:"response_status"`
	ResponseHeaders     json.RawMessage         `json:"response_headers"`
	ResponseBody        []byte                  `json:"response_body,omitempty"`
	ResponseContentType string                  `json:"response_content_type,omitempty"`
	Stream              bool                    `json:"stream"`
	UserAgent           string                  `json:"user_agent,omitempty"`
	IPAddress           string                  `json:"ip_address,omitempty"`
	DurationMs          int64                   `json:"duration_ms"`
	CreatedAt           time.Time               `json:"created_at"`
	User                *RequestRecordUser      `json:"user,omitempty"`
	APIKey              *RequestRecordAPIKey    `json:"api_key,omitempty"`
	Account             *RequestRecordReference `json:"account,omitempty"`
	Group               *RequestRecordReference `json:"group,omitempty"`
}

// RequestRecordUser is the safe user summary shown on the admin request log.
// It deliberately contains no balance, credentials, or other account data.
type RequestRecordUser struct {
	ID        int64      `json:"id"`
	Email     string     `json:"email"`
	Username  string     `json:"username,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// RequestRecordAPIKey is the safe API key summary shown on the admin request log.
// The secret key value is never loaded into this response.
type RequestRecordAPIKey struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	UserID int64  `json:"user_id"`
}

type RequestRecordReference struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type RequestRecordFilter struct {
	Page     int
	PageSize int
	// IncludePayload asks the list query to include headers and bodies. The
	// regular admin list leaves these large fields out; detail, conversation,
	// and export views opt in explicitly.
	IncludePayload bool
	RequestID      string
	Method         string
	Path           string
	Model          string
	StatusCode     *int
	UserID         *int64
	APIKeyID       *int64
	GroupID        *int64
	StartTime      *time.Time
	EndTime        *time.Time
}

type RequestRecordPage struct {
	Items    []*RequestRecord
	Total    int64
	Page     int
	PageSize int
}

// RequestRecordRepository persists complete gateway request/response pairs.
type RequestRecordRepository interface {
	Insert(ctx context.Context, record *RequestRecord) (int64, error)
	GetByID(ctx context.Context, id int64) (*RequestRecord, error)
	List(ctx context.Context, filter RequestRecordFilter) (*RequestRecordPage, error)
}

type RequestRecordService struct {
	repo RequestRecordRepository
}

func NewRequestRecordService(repo RequestRecordRepository) *RequestRecordService {
	return &RequestRecordService{repo: repo}
}

func (s *RequestRecordService) Record(ctx context.Context, record *RequestRecord) error {
	if s == nil || s.repo == nil || record == nil {
		return nil
	}
	_, err := s.repo.Insert(ctx, record)
	return err
}

// RecordAsync keeps gateway response latency independent of the export store.
// The short timeout prevents a stalled database from retaining request data
// forever during shutdown or a transient database outage.
func (s *RequestRecordService) RecordAsync(record *RequestRecord) {
	if s == nil || s.repo == nil || record == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.Record(ctx, record)
	}()
}

func (s *RequestRecordService) GetByID(ctx context.Context, id int64) (*RequestRecord, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.GetByID(ctx, id)
}

func (s *RequestRecordService) List(ctx context.Context, filter RequestRecordFilter) (*RequestRecordPage, error) {
	if s == nil || s.repo == nil {
		return &RequestRecordPage{Items: []*RequestRecord{}, Page: 1, PageSize: 20}, nil
	}
	return s.repo.List(ctx, filter)
}
