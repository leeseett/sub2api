package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
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
	stream := isStreamRequest(requestBody) || strings.Contains(strings.ToLower(recorder.Header().Get("Content-Type")), "text/event-stream")
	responseBody := recorder.capturedBody()
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
		ResponseBody:        responseBody,
		ResponseContentType: recorder.Header().Get("Content-Type"),
		Stream:              stream,
		UserAgent:           c.Request.UserAgent(),
		IPAddress:           c.ClientIP(),
		DurationMs:          time.Since(started).Milliseconds(),
		CreatedAt:           started,
	}
	if stream {
		// Compact only after the response has left the gateway. The wire bytes
		// have already been captured, so this preserves the final-result-only
		// storage policy without putting SSE parsing on the user request path.
		records.RecordAsync(record, func(record *service.RequestRecord) {
			record.ResponseBody = compactStreamResponse(record.ResponseBody)
		})
	} else {
		records.RecordAsync(record)
	}
}

// compactStreamResponse turns a potentially very large SSE transcript into a
// single final response object. The wire stream remains untouched for the
// caller; only the copy persisted to request_records is compacted.
func compactStreamResponse(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var (
		lastPayload    map[string]any
		terminal       map[string]any
		openAI         = newOpenAIStreamAccumulator()
		anthropic      = newAnthropicStreamAccumulator()
		genericText    strings.Builder
		parsedPayloads int
	)
	for _, frame := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n", "\n"), "\r", "\n"), "\n\n") {
		data := sseFrameData(frame)
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		parsedPayloads++
		lastPayload = payload
		eventType := strings.ToLower(stringValue(payload["type"]))
		if strings.Contains(eventType, "failed") || strings.Contains(eventType, "error") ||
			eventType == "response.completed" || eventType == "response.done" || eventType == "message_stop" {
			terminal = payload
		}
		openAI.add(payload)
		anthropic.add(payload)
		if delta := stringValue(payload["delta"]); delta != "" && strings.Contains(eventType, "output_text") {
			genericText.WriteString(delta)
		}
	}
	if parsedPayloads == 0 {
		return body
	}
	if terminal != nil {
		eventType := strings.ToLower(stringValue(terminal["type"]))
		if eventType == "response.completed" || eventType == "response.done" {
			if response, ok := terminal["response"].(map[string]any); ok && len(response) > 0 {
				return marshalStreamPayload(response, body)
			}
		}
		if strings.Contains(eventType, "failed") || strings.Contains(eventType, "error") {
			return marshalStreamPayload(terminal, body)
		}
	}
	if result, ok := openAI.result(); ok {
		return marshalStreamPayload(result, body)
	}
	if result, ok := anthropic.result(); ok {
		return marshalStreamPayload(result, body)
	}
	if genericText.Len() > 0 {
		return marshalStreamPayload(map[string]any{"output_text": genericText.String()}, body)
	}
	if lastPayload != nil {
		return marshalStreamPayload(lastPayload, body)
	}
	return body
}

func sseFrameData(frame string) string {
	var data []string
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if value != "" {
				data = append(data, value)
			}
		}
	}
	return strings.Join(data, "\n")
}

func marshalStreamPayload(payload map[string]any, fallback []byte) []byte {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fallback
	}
	return encoded
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

type openAIStreamChoice struct {
	index        int
	role         string
	content      strings.Builder
	finishReason any
	seen         bool
}

type openAIStreamAccumulator struct {
	base    map[string]any
	choices map[int]*openAIStreamChoice
	usage   any
}

func newOpenAIStreamAccumulator() *openAIStreamAccumulator {
	return &openAIStreamAccumulator{base: make(map[string]any), choices: make(map[int]*openAIStreamChoice)}
}

func (a *openAIStreamAccumulator) add(payload map[string]any) {
	choices, ok := payload["choices"].([]any)
	if !ok {
		return
	}
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		index := int(numberValue(choice["index"]))
		item := a.choices[index]
		if item == nil {
			item = &openAIStreamChoice{index: index}
			a.choices[index] = item
		}
		item.seen = true
		if delta, ok := choice["delta"].(map[string]any); ok {
			if role := stringValue(delta["role"]); role != "" {
				item.role = role
			}
			if content := stringValue(delta["content"]); content != "" {
				item.content.WriteString(content)
			}
		} else if message, ok := choice["message"].(map[string]any); ok {
			if role := stringValue(message["role"]); role != "" {
				item.role = role
			}
			if content := stringValue(message["content"]); content != "" {
				item.content.WriteString(content)
			}
		}
		if reason, exists := choice["finish_reason"]; exists && reason != nil {
			item.finishReason = reason
		}
	}
	for _, key := range []string{"id", "object", "created", "model", "system_fingerprint"} {
		if value, exists := payload[key]; exists && a.base[key] == nil {
			a.base[key] = value
		}
	}
	if usage, exists := payload["usage"]; exists {
		a.usage = usage
	}
}

func (a *openAIStreamAccumulator) result() (map[string]any, bool) {
	if len(a.choices) == 0 {
		return nil, false
	}
	indices := make([]int, 0, len(a.choices))
	for index := range a.choices {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	result := make(map[string]any, len(a.base)+2)
	for key, value := range a.base {
		if key == "object" {
			if object, ok := value.(string); ok {
				value = strings.TrimSuffix(object, ".chunk")
			}
		}
		result[key] = value
	}
	items := make([]any, 0, len(indices))
	for _, index := range indices {
		choice := a.choices[index]
		message := map[string]any{"role": choice.role, "content": choice.content.String()}
		item := map[string]any{"index": choice.index, "message": message}
		if choice.finishReason != nil {
			item["finish_reason"] = choice.finishReason
		}
		items = append(items, item)
	}
	result["choices"] = items
	if a.usage != nil {
		result["usage"] = a.usage
	}
	return result, true
}

type anthropicStreamAccumulator struct {
	message map[string]any
	text    strings.Builder
	usage   any
	seen    bool
}

func newAnthropicStreamAccumulator() *anthropicStreamAccumulator {
	return &anthropicStreamAccumulator{}
}

func (a *anthropicStreamAccumulator) add(payload map[string]any) {
	eventType := strings.ToLower(stringValue(payload["type"]))
	if eventType == "message_start" {
		if message, ok := payload["message"].(map[string]any); ok {
			a.message = message
			a.seen = true
		}
		return
	}
	if eventType == "content_block_delta" {
		if delta, ok := payload["delta"].(map[string]any); ok {
			if text := stringValue(delta["text"]); text != "" {
				a.text.WriteString(text)
			}
		}
		return
	}
	if eventType == "message_delta" {
		a.seen = true
		if usage, ok := payload["usage"]; ok {
			a.usage = usage
		}
		if delta, ok := payload["delta"].(map[string]any); ok && a.message != nil {
			if reason, ok := delta["stop_reason"]; ok {
				a.message["stop_reason"] = reason
			}
		}
	}
}

func (a *anthropicStreamAccumulator) result() (map[string]any, bool) {
	if !a.seen || a.message == nil || a.text.Len() == 0 {
		return nil, false
	}
	result := make(map[string]any, len(a.message)+1)
	for key, value := range a.message {
		result[key] = value
	}
	result["content"] = []any{map[string]any{"type": "text", "text": a.text.String()}}
	if a.usage != nil {
		result["usage"] = a.usage
	}
	return result, true
}

func numberValue(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case json.Number:
		parsed, _ := strconv.ParseFloat(string(number), 64)
		return parsed
	default:
		return 0
	}
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
