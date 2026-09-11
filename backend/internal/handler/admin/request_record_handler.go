package admin

import (
	"encoding/base64"
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RequestRecordHandler exposes complete gateway request/response records.
type RequestRecordHandler struct {
	service *service.RequestRecordService
}

func NewRequestRecordHandler(recordService *service.RequestRecordService) *RequestRecordHandler {
	return &RequestRecordHandler{service: recordService}
}

func (h *RequestRecordHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Request record service is unavailable")
		return
	}
	filter, err := parseRequestRecordFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	page, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, page.Items, page.Total, page.Page, page.PageSize)
}

func (h *RequestRecordHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Request record service is unavailable")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid request record id")
		return
	}
	record, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if record == nil {
		response.NotFound(c, "Request record not found")
		return
	}
	response.Success(c, record)
}

// Export writes CSV in pages, so exporting a large dataset does not require a
// second in-memory copy of all records. Bodies are base64 to preserve binary
// request/response payloads exactly.
func (h *RequestRecordHandler) Export(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Request record service is unavailable")
		return
	}
	filter, err := parseRequestRecordFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter.Page = 1
	filter.PageSize = 5000
	filter.IncludePayload = true
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="request-records.csv"`)
	w := csv.NewWriter(c.Writer)
	if err := w.Write([]string{
		"id", "request_id", "client_request_id", "user_id", "api_key_id", "account_id", "group_id",
		"user_email", "api_key_name", "account_name", "group_name", "model", "method", "path", "query_string",
		"request_headers", "request_body_base64", "request_content_type",
		"response_status", "response_headers", "response_body_base64", "response_content_type", "stream",
		"duration_ms", "user_agent", "ip_address", "created_at",
	}); err != nil {
		return
	}
	for {
		page, listErr := h.service.List(c.Request.Context(), filter)
		if listErr != nil {
			return
		}
		for _, record := range page.Items {
			if writeErr := w.Write(requestRecordCSVRow(record)); writeErr != nil {
				return
			}
		}
		if len(page.Items) == 0 || filter.Page*filter.PageSize >= int(page.Total) {
			break
		}
		filter.Page++
	}
	w.Flush()
}

func parseRequestRecordFilter(c *gin.Context) (service.RequestRecordFilter, error) {
	page, pageSize := response.ParsePagination(c)
	filter := service.RequestRecordFilter{
		Page:           page,
		PageSize:       pageSize,
		IncludePayload: false,
		RequestID:      strings.TrimSpace(c.Query("request_id")),
		Method:         strings.TrimSpace(c.Query("method")),
		Path:           strings.TrimSpace(c.Query("path")),
		Model:          strings.TrimSpace(c.Query("model")),
	}
	if raw := strings.TrimSpace(c.Query("include_payload")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, invalidFilter("include_payload")
		}
		filter.IncludePayload = value
	}
	if raw := strings.TrimSpace(c.Query("conversation_only")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, invalidFilter("conversation_only")
		}
		filter.ConversationOnly = value
		if value {
			filter.IncludePayload = true
		}
	}
	for name, target := range map[string]**int64{
		"user_id":    &filter.UserID,
		"api_key_id": &filter.APIKeyID,
		"group_id":   &filter.GroupID,
	} {
		if raw := strings.TrimSpace(c.Query(name)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				return filter, invalidFilter(name)
			}
			*target = &value
		}
	}
	if raw := strings.TrimSpace(c.Query("status_code")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 100 || value > 599 {
			return filter, invalidFilter("status_code")
		}
		filter.StatusCode = &value
	}
	var err error
	filter.StartTime, err = parseRequestRecordTime(c.Query("start_time"))
	if err != nil {
		return filter, invalidFilter("start_time")
	}
	filter.EndTime, err = parseRequestRecordTime(c.Query("end_time"))
	if err != nil {
		return filter, invalidFilter("end_time")
	}
	return filter, nil
}

func parseRequestRecordTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse("2006-01-02", value)
	}
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func invalidFilter(name string) error {
	return &requestRecordFilterError{name: name}
}

type requestRecordFilterError struct{ name string }

func (e *requestRecordFilterError) Error() string { return "Invalid " + e.name }

func requestRecordCSVRow(record *service.RequestRecord) []string {
	if record == nil {
		return nil
	}
	return []string{
		strconv.FormatInt(record.ID, 10), record.RequestID, record.ClientRequestID, optionalInt64(record.UserID),
		optionalInt64(record.APIKeyID), optionalInt64(record.AccountID), optionalInt64(record.GroupID), requestRecordUserEmail(record),
		requestRecordAPIKeyName(record), requestRecordReferenceName(record.Account), requestRecordReferenceName(record.Group), record.Model, record.Method,
		record.Path, record.QueryString, string(record.RequestHeaders), base64.StdEncoding.EncodeToString(record.RequestBody),
		record.RequestContentType, strconv.Itoa(record.ResponseStatus), string(record.ResponseHeaders),
		base64.StdEncoding.EncodeToString(record.ResponseBody), record.ResponseContentType, strconv.FormatBool(record.Stream),
		strconv.FormatInt(record.DurationMs, 10), record.UserAgent, record.IPAddress, record.CreatedAt.Format(time.RFC3339Nano),
	}
}

func requestRecordUserEmail(record *service.RequestRecord) string {
	if record == nil || record.User == nil {
		return ""
	}
	return record.User.Email
}

func requestRecordAPIKeyName(record *service.RequestRecord) string {
	if record == nil || record.APIKey == nil {
		return ""
	}
	return record.APIKey.Name
}

func requestRecordReferenceName(reference *service.RequestRecordReference) string {
	if reference == nil {
		return ""
	}
	return reference.Name
}

func optionalInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
