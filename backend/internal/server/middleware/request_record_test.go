package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestRecordMemoryRepo struct {
	mu      sync.Mutex
	records []*service.RequestRecord
}

func (r *requestRecordMemoryRepo) Insert(_ context.Context, record *service.RequestRecord) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return int64(len(r.records)), nil
}

func (r *requestRecordMemoryRepo) GetByID(context.Context, int64) (*service.RequestRecord, error) {
	return nil, nil
}

func (r *requestRecordMemoryRepo) List(context.Context, service.RequestRecordFilter) (*service.RequestRecordPage, error) {
	return nil, nil
}

func (r *requestRecordMemoryRepo) first() *service.RequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.records) == 0 {
		return nil
	}
	return r.records[0]
}

func TestRequestRecordMiddlewareCapturesWireBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &requestRecordMemoryRepo{}
	router := gin.New()
	router.Use(RequestRecordMiddleware(service.NewRequestRecordService(repo)))
	router.POST("/capture", func(c *gin.Context) {
		body, err := c.GetRawData()
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5","prompt":"hello"}`, string(body))
		c.Data(201, "application/json", []byte(`{"ok":true}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/capture", strings.NewReader(`{"model":"gpt-5","prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "request-record-test/1.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 201, w.Code)

	var record *service.RequestRecord
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		record = repo.first()
		if record != nil {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	require.NotNil(t, record)
	require.Equal(t, []byte(`{"model":"gpt-5","prompt":"hello"}`), record.RequestBody)
	require.Equal(t, []byte(`{"ok":true}`), record.ResponseBody)
	require.Equal(t, 201, record.ResponseStatus)
	require.Equal(t, "/capture", record.Path)
	require.Equal(t, "gpt-5", record.Model)
	require.Equal(t, "request-record-test/1.0", record.UserAgent)
	require.Equal(t, "192.0.2.1", record.IPAddress)
}

func TestCompactStreamResponseKeepsFinalOpenAIResultOnly(t *testing.T) {
	body := []byte("data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":2}}\n\n" +
		"data: [DONE]\n\n")
	result := compactStreamResponse(body)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(result, &payload))
	require.NotContains(t, string(result), "chat.completion.chunk")
	require.Equal(t, "Hello", payload["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"])
	require.Equal(t, "stop", payload["choices"].([]any)[0].(map[string]any)["finish_reason"])
}

func TestRequestRecordMiddlewareCompactsSSEResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &requestRecordMemoryRepo{}
	router := gin.New()
	router.Use(RequestRecordMiddleware(service.NewRequestRecordService(repo)))
	router.POST("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
		_, _ = c.Writer.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	})
	req := httptest.NewRequest(http.MethodPost, "/stream", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), "[DONE]")
	var record *service.RequestRecord
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		record = repo.first()
		if record != nil {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	require.NotNil(t, record)
	require.NotContains(t, string(record.ResponseBody), "[DONE]")
	require.Contains(t, string(record.ResponseBody), "hello world")
}
