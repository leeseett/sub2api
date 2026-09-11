package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactRequestRecordConversationDropsRawPayloads(t *testing.T) {
	record := &RequestRecord{
		RequestBody:     []byte(`{"system":"long system prompt","messages":[{"role":"system","content":"another system block"},{"role":"user","content":"hello"},{"role":"assistant","content":"old answer"}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"world"}}]}`),
		RequestHeaders:  []byte(`{"x-test":["1"]}`),
		ResponseHeaders: []byte(`{"content-type":["application/json"]}`),
	}
	compactRequestRecordConversation(record)
	require.Equal(t, "long system prompt\n\nanother system block", record.ConversationSystem)
	require.Equal(t, "hello", record.ConversationRequest)
	require.Equal(t, "world", record.ConversationResponse)
	require.Nil(t, record.RequestBody)
	require.Nil(t, record.ResponseBody)
	require.Nil(t, record.RequestHeaders)
	require.Nil(t, record.ResponseHeaders)
}

func TestConversationRequestPartsSeparatesResponsesInstructions(t *testing.T) {
	system, user := conversationRequestParts([]byte(`{"instructions":"system rules","input":[{"role":"developer","content":"developer rules"},{"role":"user","content":"hello"}]}`))
	require.Equal(t, "system rules\n\ndeveloper rules", system)
	require.Equal(t, "hello", user)
}
