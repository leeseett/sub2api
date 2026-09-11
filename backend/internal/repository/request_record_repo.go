package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestRecordRepository struct {
	db *sql.DB
}

func NewRequestRecordRepository(db *sql.DB) service.RequestRecordRepository {
	return &requestRecordRepository{db: db}
}

const requestRecordColumns = `
 rr.id, rr.request_id, rr.client_request_id, rr.user_id, rr.api_key_id, rr.account_id, rr.group_id,
 rr.model, rr.method, rr.path, rr.query_string, rr.request_headers, rr.request_body, rr.request_content_type,
 rr.response_status, rr.response_headers, rr.response_body, rr.response_content_type,
 rr.stream, rr.user_agent, rr.ip_address, rr.duration_ms, rr.created_at,
 u.email, u.username, u.deleted_at, ak.name, a.name, grp.name`

// requestRecordSummaryColumns deliberately excludes the potentially large
// headers and bodies. The admin list only needs metadata; GetByID and the
// explicit payload views still use requestRecordColumns.
const requestRecordSummaryColumns = `
 rr.id, rr.request_id, rr.client_request_id, rr.user_id, rr.api_key_id, rr.account_id, rr.group_id,
 rr.model, rr.method, rr.path, rr.query_string, rr.request_content_type,
 rr.response_status, rr.response_content_type, rr.stream, rr.user_agent, rr.ip_address,
 rr.duration_ms, rr.created_at,
 u.email, u.username, u.deleted_at, ak.name, a.name, grp.name`

const requestRecordFromSQL = `
 request_records rr
 LEFT JOIN users u ON u.id = rr.user_id
 LEFT JOIN api_keys ak ON ak.id = rr.api_key_id
 LEFT JOIN accounts a ON a.id = rr.account_id
 LEFT JOIN groups grp ON grp.id = rr.group_id`

func (r *requestRecordRepository) Insert(ctx context.Context, record *service.RequestRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil request record repository")
	}
	if record == nil {
		return 0, fmt.Errorf("nil request record")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	requestHeaders := normalizeHeaders(record.RequestHeaders)
	responseHeaders := normalizeHeaders(record.ResponseHeaders)
	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO request_records (
 request_id, client_request_id, user_id, api_key_id, account_id, group_id, model,
 method, path, query_string, request_headers, request_body, request_content_type,
 response_status, response_headers, response_body, response_content_type,
 stream, user_agent, ip_address, duration_ms, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15::jsonb,$16,$17,$18,$19,$20,$21,$22)
RETURNING id`,
		requestRecordNullString(record.RequestID), requestRecordNullString(record.ClientRequestID), nullableInt64(record.UserID),
		nullableInt64(record.APIKeyID), nullableInt64(record.AccountID), nullableInt64(record.GroupID),
		requestRecordNullString(record.Model), record.Method, record.Path, requestRecordNullString(record.QueryString), requestHeaders, record.RequestBody,
		requestRecordNullString(record.RequestContentType), record.ResponseStatus, responseHeaders, record.ResponseBody,
		requestRecordNullString(record.ResponseContentType), record.Stream, requestRecordNullString(record.UserAgent),
		requestRecordNullString(record.IPAddress), record.DurationMs, record.CreatedAt,
	).Scan(&id)
	return id, err
}

func (r *requestRecordRepository) GetByID(ctx context.Context, id int64) (*service.RequestRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil request record repository")
	}
	row := r.db.QueryRowContext(ctx, `SELECT`+requestRecordColumns+` FROM`+requestRecordFromSQL+` WHERE rr.id = $1`, id)
	return scanRequestRecord(row)
}

func (r *requestRecordRepository) List(ctx context.Context, filter service.RequestRecordFilter) (*service.RequestRecordPage, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil request record repository")
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 5000 {
		pageSize = 5000
	}
	where := make([]string, 0, 10)
	args := make([]any, 0, 10)
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if v := strings.TrimSpace(filter.RequestID); v != "" {
		add("rr.request_id = $%d", v)
	}
	if v := strings.TrimSpace(filter.Method); v != "" {
		add("rr.method = $%d", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(filter.Path); v != "" {
		add("rr.path = $%d", v)
	}
	if v := strings.TrimSpace(filter.Model); v != "" {
		add("rr.model = $%d", v)
	}
	if filter.StatusCode != nil {
		add("rr.response_status = $%d", *filter.StatusCode)
	}
	if filter.UserID != nil {
		add("rr.user_id = $%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add("rr.api_key_id = $%d", *filter.APIKeyID)
	}
	if filter.GroupID != nil {
		add("rr.group_id = $%d", *filter.GroupID)
	}
	if filter.StartTime != nil {
		add("rr.created_at >= $%d", *filter.StartTime)
	}
	if filter.EndTime != nil {
		add("rr.created_at < $%d", *filter.EndTime)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+requestRecordFromSQL+whereSQL, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	columns := requestRecordSummaryColumns
	if filter.IncludePayload || filter.ConversationOnly {
		columns = requestRecordColumns
	}
	rows, err := r.db.QueryContext(ctx, `SELECT`+columns+` FROM`+requestRecordFromSQL+whereSQL+` ORDER BY rr.created_at DESC, rr.id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*service.RequestRecord, 0)
	for rows.Next() {
		var item *service.RequestRecord
		var scanErr error
		if filter.IncludePayload || filter.ConversationOnly {
			item, scanErr = scanRequestRecord(rows)
		} else {
			item, scanErr = scanRequestRecordSummary(rows)
		}
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.RequestRecordPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func scanRequestRecordSummary(row rowScanner) (*service.RequestRecord, error) {
	var (
		record                                                  service.RequestRecord
		requestID, clientRequestID                              sql.NullString
		userID, apiKeyID, accountID, groupID                    sql.NullInt64
		model, queryString, requestContentType                  sql.NullString
		userAgent, ipAddress, responseContentType               sql.NullString
		userEmail, username, apiKeyName, accountName, groupName sql.NullString
		userDeletedAt                                           sql.NullTime
	)
	err := row.Scan(
		&record.ID, &requestID, &clientRequestID, &userID, &apiKeyID, &accountID, &groupID,
		&model, &record.Method, &record.Path, &queryString, &requestContentType,
		&record.ResponseStatus, &responseContentType, &record.Stream, &userAgent, &ipAddress,
		&record.DurationMs, &record.CreatedAt,
		&userEmail, &username, &userDeletedAt, &apiKeyName, &accountName, &groupName,
	)
	if err != nil {
		return nil, err
	}
	populateRequestRecordReferences(&record, requestID, clientRequestID, userID, apiKeyID, accountID, groupID,
		model, queryString, requestContentType, userAgent, ipAddress, responseContentType,
		userEmail, username, userDeletedAt, apiKeyName, accountName, groupName)
	return &record, nil
}

func scanRequestRecord(row rowScanner) (*service.RequestRecord, error) {
	var (
		record                                                  service.RequestRecord
		requestID, clientRequestID                              sql.NullString
		userID, apiKeyID, accountID, groupID                    sql.NullInt64
		model, queryString, requestContentType                  sql.NullString
		userAgent, ipAddress, responseContentType               sql.NullString
		userEmail, username, apiKeyName, accountName, groupName sql.NullString
		userDeletedAt                                           sql.NullTime
		requestHeaders, responseHeaders                         []byte
		requestBody, responseBody                               []byte
	)
	err := row.Scan(
		&record.ID, &requestID, &clientRequestID, &userID, &apiKeyID, &accountID, &groupID,
		&model, &record.Method, &record.Path, &queryString, &requestHeaders, &requestBody, &requestContentType,
		&record.ResponseStatus, &responseHeaders, &responseBody, &responseContentType,
		&record.Stream, &userAgent, &ipAddress, &record.DurationMs, &record.CreatedAt,
		&userEmail, &username, &userDeletedAt, &apiKeyName, &accountName, &groupName,
	)
	if err != nil {
		return nil, err
	}
	record.RequestHeaders = normalizeHeaders(requestHeaders)
	record.ResponseHeaders = normalizeHeaders(responseHeaders)
	record.RequestBody, record.ResponseBody = append([]byte(nil), requestBody...), append([]byte(nil), responseBody...)
	populateRequestRecordReferences(&record, requestID, clientRequestID, userID, apiKeyID, accountID, groupID,
		model, queryString, requestContentType, userAgent, ipAddress, responseContentType,
		userEmail, username, userDeletedAt, apiKeyName, accountName, groupName)
	return &record, nil
}

func populateRequestRecordReferences(record *service.RequestRecord, requestID, clientRequestID sql.NullString,
	userID, apiKeyID, accountID, groupID sql.NullInt64, model, queryString, requestContentType,
	userAgent, ipAddress, responseContentType, userEmail, username sql.NullString, userDeletedAt sql.NullTime,
	apiKeyName, accountName, groupName sql.NullString) {
	record.RequestID, record.ClientRequestID = requestID.String, clientRequestID.String
	record.Model = model.String
	record.QueryString, record.RequestContentType = queryString.String, requestContentType.String
	record.ResponseContentType = responseContentType.String
	record.UserAgent, record.IPAddress = userAgent.String, ipAddress.String
	record.RequestHeaders = normalizeHeaders(record.RequestHeaders)
	record.ResponseHeaders = normalizeHeaders(record.ResponseHeaders)
	record.UserID = nullableInt64Ptr(userID)
	record.APIKeyID = nullableInt64Ptr(apiKeyID)
	record.AccountID = nullableInt64Ptr(accountID)
	record.GroupID = nullableInt64Ptr(groupID)
	if userID.Valid && userEmail.Valid {
		record.User = &service.RequestRecordUser{ID: userID.Int64, Email: userEmail.String, Username: username.String}
		if userDeletedAt.Valid {
			record.User.DeletedAt = &userDeletedAt.Time
		}
	}
	if apiKeyID.Valid && apiKeyName.Valid {
		record.APIKey = &service.RequestRecordAPIKey{ID: apiKeyID.Int64, Name: apiKeyName.String}
		if userID.Valid {
			record.APIKey.UserID = userID.Int64
		}
	}
	if accountID.Valid && accountName.Valid {
		record.Account = &service.RequestRecordReference{ID: accountID.Int64, Name: accountName.String}
	}
	if groupID.Valid && groupName.Valid {
		record.Group = &service.RequestRecordReference{ID: groupID.Int64, Name: groupName.String}
	}
}

func normalizeHeaders(value []byte) []byte {
	if len(value) == 0 || !json.Valid(value) {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
}

func requestRecordNullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

var _ service.RequestRecordRepository = (*requestRecordRepository)(nil)
