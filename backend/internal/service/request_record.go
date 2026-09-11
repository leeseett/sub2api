package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// RequestRecord is the complete wire-level gateway request/response pair.
// Bodies use []byte so binary endpoints (audio, images, etc.) are preserved;
// encoding/json exposes them as base64 when returned by the admin API.
type RequestRecord struct {
	ID                   int64                   `json:"id"`
	RequestID            string                  `json:"request_id,omitempty"`
	ClientRequestID      string                  `json:"client_request_id,omitempty"`
	UserID               *int64                  `json:"user_id,omitempty"`
	APIKeyID             *int64                  `json:"api_key_id,omitempty"`
	AccountID            *int64                  `json:"account_id,omitempty"`
	GroupID              *int64                  `json:"group_id,omitempty"`
	Model                string                  `json:"model,omitempty"`
	Method               string                  `json:"method"`
	Path                 string                  `json:"path"`
	QueryString          string                  `json:"query_string,omitempty"`
	RequestHeaders       json.RawMessage         `json:"request_headers,omitempty"`
	RequestBody          []byte                  `json:"request_body,omitempty"`
	ConversationSystem   string                  `json:"conversation_system,omitempty"`
	ConversationRequest  string                  `json:"conversation_request,omitempty"`
	ConversationResponse string                  `json:"conversation_response,omitempty"`
	RequestContentType   string                  `json:"request_content_type,omitempty"`
	ResponseStatus       int                     `json:"response_status"`
	ResponseHeaders      json.RawMessage         `json:"response_headers,omitempty"`
	ResponseBody         []byte                  `json:"response_body,omitempty"`
	ResponseContentType  string                  `json:"response_content_type,omitempty"`
	Stream               bool                    `json:"stream"`
	UserAgent            string                  `json:"user_agent,omitempty"`
	IPAddress            string                  `json:"ip_address,omitempty"`
	DurationMs           int64                   `json:"duration_ms"`
	CreatedAt            time.Time               `json:"created_at"`
	User                 *RequestRecordUser      `json:"user,omitempty"`
	APIKey               *RequestRecordAPIKey    `json:"api_key,omitempty"`
	Account              *RequestRecordReference `json:"account,omitempty"`
	Group                *RequestRecordReference `json:"group,omitempty"`
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
	IncludePayload   bool
	ConversationOnly bool
	RequestID        string
	Method           string
	Path             string
	Model            string
	StatusCode       *int
	UserID           *int64
	APIKeyID         *int64
	GroupID          *int64
	StartTime        *time.Time
	EndTime          *time.Time
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
	page, err := s.repo.List(ctx, filter)
	if err != nil || !filter.ConversationOnly {
		return page, err
	}
	for _, record := range page.Items {
		compactRequestRecordConversation(record)
	}
	return page, nil
}

const (
	conversationSystemLimit   = 8192
	conversationRequestLimit  = 65536
	conversationResponseLimit = 131072
)

func compactRequestRecordConversation(record *RequestRecord) {
	if record == nil {
		return
	}
	record.ConversationSystem, record.ConversationRequest = conversationRequestParts(record.RequestBody)
	record.ConversationResponse = conversationResponseText(record.ResponseBody)
	record.ConversationSystem = truncateConversationText(record.ConversationSystem, conversationSystemLimit)
	record.ConversationRequest = truncateConversationText(record.ConversationRequest, conversationRequestLimit)
	record.ConversationResponse = truncateConversationText(record.ConversationResponse, conversationResponseLimit)
	// The conversation view never needs raw headers/bodies. Detail and export
	// requests still use the original complete record.
	record.RequestHeaders = nil
	record.ResponseHeaders = nil
	record.RequestBody = nil
	record.ResponseBody = nil
}

func conversationRequestParts(body []byte) (string, string) {
	payload := decodeConversationJSON(body)
	object, ok := payload.(map[string]any)
	if !ok {
		return "", string(body)
	}
	var system []string
	if value, exists := object["system"]; exists {
		system = append(system, conversationValueText(value))
	}
	var user []string
	if messages, ok := object["messages"].([]any); ok {
		for _, value := range messages {
			message, ok := value.(map[string]any)
			if !ok {
				continue
			}
			role := strings.ToLower(conversationStringValue(message["role"]))
			content := conversationValueText(firstConversationValue(message, "content", "parts", "text"))
			if role == "system" {
				system = append(system, content)
			} else if role == "user" || role == "" {
				user = append(user, content)
			}
		}
	}
	if contents, ok := object["contents"].([]any); ok && len(user) == 0 {
		for _, value := range contents {
			message, ok := value.(map[string]any)
			if !ok {
				user = append(user, conversationValueText(value))
				continue
			}
			role := strings.ToLower(conversationStringValue(message["role"]))
			if role == "system" {
				system = append(system, conversationValueText(message["content"]))
			} else {
				user = append(user, conversationValueText(firstConversationValue(message, "content", "parts", "text")))
			}
		}
	}
	if len(user) == 0 {
		for _, key := range []string{"input", "prompt"} {
			if value, exists := object[key]; exists {
				user = append(user, conversationValueText(value))
				break
			}
		}
	}
	return joinConversationText(system), joinConversationText(user)
}

func conversationResponseText(body []byte) string {
	payload := decodeConversationJSON(body)
	if object, ok := payload.(map[string]any); ok {
		if response, ok := object["response"].(map[string]any); ok {
			payload = response
			object = response
		}
		if choices, ok := object["choices"].([]any); ok {
			var values []string
			for _, value := range choices {
				choice, ok := value.(map[string]any)
				if !ok {
					continue
				}
				message := firstConversationValue(choice, "message", "delta", "content", "text")
				if nested, ok := message.(map[string]any); ok {
					message = firstConversationValue(nested, "content", "text", "output_text")
				}
				values = append(values, conversationValueText(message))
			}
			if text := joinConversationText(values); text != "" {
				return text
			}
		}
		for _, key := range []string{"output_text", "output", "content", "text"} {
			if value, exists := object[key]; exists {
				if text := conversationValueText(value); text != "" {
					return text
				}
			}
		}
	}
	return string(body)
}

func decodeConversationJSON(body []byte) any {
	var payload any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return nil
	}
	return payload
}

func firstConversationValue(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := object[key]; exists {
			return value
		}
	}
	return nil
}

func conversationValueText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	if values, ok := value.([]any); ok {
		var parts []string
		for _, item := range values {
			if text := conversationValueText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"text", "content", "output_text", "parts"} {
			if nested, exists := object[key]; exists {
				if text := conversationValueText(nested); text != "" {
					return text
				}
			}
		}
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func conversationStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func joinConversationText(values []string) string {
	var nonEmpty []string
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			nonEmpty = append(nonEmpty, value)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

func truncateConversationText(value string, limit int) string {
	valueRunes := []rune(value)
	if len(valueRunes) <= limit {
		return value
	}
	return string(valueRunes[:limit]) + "\n…"
}
