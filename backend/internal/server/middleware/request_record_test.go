package middleware

import (
	"context"
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
