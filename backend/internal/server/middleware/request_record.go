package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// requestRecordWriter keeps the normal Gin writer behavior while retaining the
// exact bytes written to the client. Embedding gin.ResponseWriter preserves
// Flush/Hijack/Pusher behavior for streaming and websocket endpoints.
type requestRecordWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *requestRecordWriter) Write(body []byte) (int, error) {
	_, _ = w.body.Write(body)
	return w.ResponseWriter.Write(body)
}

func (w *requestRecordWriter) WriteString(body string) (int, error) {
	_, _ = w.body.WriteString(body)
	return w.ResponseWriter.WriteString(body)
}

func (w *requestRecordWriter) capturedBody() []byte {
	return append([]byte(nil), w.body.Bytes()...)
}

// RequestRecordMiddleware records requests mounted on a gateway route group.
// It deliberately runs before authentication so rejected requests are retained
// too; identity fields are filled after downstream middleware has run.
func RequestRecordMiddleware(records *service.RequestRecordService) gin.HandlerFunc {
	return func(c *gin.Context) {
		captureRequestRecord(c, records, func() { c.Next() })
	}
}

// RootGatewayRequestRecordMiddleware covers the unprefixed OpenAI-compatible
// aliases. It is installed once on the server before route registration so
// authentication failures on those aliases are recorded as well.
func RootGatewayRequestRecordMiddleware(records *service.RequestRecordService, maxBodySize ...int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isRootGatewayAliasPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		var limit int64
		if len(maxBodySize) > 0 {
			limit = maxBodySize[0]
		}
		captureRequestRecordWithLimit(c, records, limit, func() { c.Next() })
	}
}

func isRootGatewayAliasPath(path string) bool {
	if path == "/antigravity/models" || path == "/models" || path == "/responses" || path == "/alpha/search" || path == "/messages/count_tokens" || path == "/chat/completions" || path == "/embeddings" || path == "/realtime" || path == "/web_search" || path == "/x_search" {
		return true
	}
	for _, prefix := range []string{"/responses/", "/images/", "/videos/", "/tts", "/stt", "/custom-voices"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func captureRequestRecord(c *gin.Context, records *service.RequestRecordService, run func()) {
	captureRequestRecordWithLimit(c, records, 0, run)
}

func captureRequestRecordWithLimit(c *gin.Context, records *service.RequestRecordService, maxBodySize int64, run func()) {
	if records == nil || c == nil || c.Request == nil {
		run()
		return
	}
	started := time.Now()
	originalWriter := c.Writer
	recorder := &requestRecordWriter{ResponseWriter: originalWriter}
	c.Writer = recorder
	requestBody, readErr := readAndRestoreBody(c, maxBodySize)
	if readErr != nil {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
	} else {
		run()
	}
	if c.Writer == recorder {
		c.Writer = originalWriter
	}

	status := recorder.Status()
	if status <= 0 {
		status = http.StatusOK
	}
	requestHeaders, _ := json.Marshal(c.Request.Header)
	responseHeaders, _ := json.Marshal(recorder.Header())
	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
	if strings.TrimSpace(requestID) == "" {
		requestID = c.GetHeader("X-Request-ID")
	}
	if strings.TrimSpace(clientRequestID) == "" {
		clientRequestID = c.GetHeader("X-Client-Request-ID")
	}
	model := strings.TrimSpace(requestmodel.FromBodyForRoute(c.Request.URL.Path, c.GetHeader("Content-Type"), requestBody))
	if model == "" {
		model = strings.TrimSpace(c.Query("model"))
	}
	record := &service.RequestRecord{
		RequestID:           strings.TrimSpace(requestID),
		ClientRequestID:     strings.TrimSpace(clientRequestID),
		UserID:              requestRecordUserID(c),
		APIKeyID:            requestRecordAPIKeyID(c),
		AccountID:           requestRecordContextID(c, ctxkey.AccountID),
		GroupID:             requestRecordGroupID(c),
		Model:               model,
		Method:              c.Request.Method,
		Path:                c.Request.URL.Path,
		QueryString:         c.Request.URL.RawQuery,
		RequestHeaders:      requestHeaders,
		RequestBody:         requestBody,
		RequestContentType:  c.GetHeader("Content-Type"),
		ResponseStatus:      status,
		ResponseHeaders:     responseHeaders,
		ResponseBody:        recorder.capturedBody(),
		ResponseContentType: recorder.Header().Get("Content-Type"),
		Stream:              isStreamRequest(requestBody) || strings.Contains(strings.ToLower(recorder.Header().Get("Content-Type")), "text/event-stream"),
		UserAgent:           c.Request.UserAgent(),
		IPAddress:           c.ClientIP(),
		DurationMs:          time.Since(started).Milliseconds(),
		CreatedAt:           started,
	}
	records.RecordAsync(record)
}

func readAndRestoreBody(c *gin.Context, maxBodySize int64) ([]byte, error) {
	request := c.Request
	if request == nil || request.Body == nil {
		return nil, nil
	}
	if maxBodySize > 0 {
		request.Body = http.MaxBytesReader(c.Writer, request.Body, maxBodySize)
	}
	body, err := io.ReadAll(request.Body)
	_ = request.Body.Close()
	if err != nil {
		return body, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func isStreamRequest(body []byte) bool {
	return bytes.Contains(bytes.ToLower(body), []byte(`"stream":true`)) ||
		bytes.Contains(bytes.ToLower(body), []byte(`"stream": true`))
}

func requestRecordContextID(c *gin.Context, key ctxkey.Key) *int64 {
	if c == nil || c.Request == nil {
		return nil
	}
	value, ok := c.Request.Context().Value(key).(int64)
	if !ok || value <= 0 {
		return nil
	}
	return &value
}

func requestRecordUserID(c *gin.Context) *int64 {
	if value := requestRecordContextID(c, ctxkey.UserID); value != nil {
		return value
	}
	if c != nil {
		if subject, ok := c.Get(string(ContextKeyUser)); ok {
			if auth, ok := subject.(AuthSubject); ok && auth.UserID > 0 {
				return &auth.UserID
			}
		}
	}
	return nil
}

func requestRecordAPIKeyID(c *gin.Context) *int64 {
	if c == nil {
		return nil
	}
	value, ok := c.Get(string(ContextKeyAPIKey))
	if !ok {
		return nil
	}
	key, ok := value.(*service.APIKey)
	if !ok || key == nil || key.ID <= 0 {
		return nil
	}
	return &key.ID
}

func requestRecordGroupID(c *gin.Context) *int64 {
	if c == nil {
		return nil
	}
	value, ok := c.Get(string(ContextKeyAPIKey))
	if !ok {
		return nil
	}
	key, ok := value.(*service.APIKey)
	if !ok || key == nil || key.GroupID == nil || *key.GroupID <= 0 {
		return nil
	}
	return key.GroupID
}

var _ gin.ResponseWriter = (*requestRecordWriter)(nil)
