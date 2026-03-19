package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormattedEventAction(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	action := formattedEventAction(event)
	assert.Equal("RESOLVED", action)

	event.Check.Status = 1
	action = formattedEventAction(event)
	assert.Equal("ALERT", action)
}

func TestChomp(t *testing.T) {
	assert := assert.New(t)

	trimNewline := chomp("hello\n")
	assert.Equal("hello", trimNewline)

	trimCarriageReturn := chomp("hello\r")
	assert.Equal("hello", trimCarriageReturn)

	trimBoth := chomp("hello\r\n")
	assert.Equal("hello", trimBoth)

	trimLots := chomp("hello\r\n\r\n\r\n")
	assert.Equal("hello", trimLots)
}

func TestEventKey(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")
	eventKey := eventKey(event)
	assert.Equal("entity1/check1", eventKey)
}

func TestEventSummary(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")
	event.Check.Output = "disk is full"

	eventKey := eventSummary(event, 100)
	assert.Equal("entity1/check1:disk is full", eventKey)

	eventKey = eventSummary(event, 5)
	assert.Equal("entity1/check1:disk ...", eventKey)
}

func TestFormattedMessage(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")
	event.Check.Output = "disk is full"
	event.Check.Status = 1
	formattedMsg := formattedMessage(event)
	assert.Equal("ALERT - entity1/check1:disk is full", formattedMsg)
}

func TestMessageColor(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	event.Check.Status = 0
	color := messageColor(event)
	assert.Equal("good", color)

	event.Check.Status = 1
	color = messageColor(event)
	assert.Equal("warning", color)

	event.Check.Status = 2
	color = messageColor(event)
	assert.Equal("attention", color)
}

func TestMessageStatus(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	event.Check.Status = 0
	status := messageStatus(event)
	assert.Equal("Resolved", status)

	event.Check.Status = 1
	status = messageStatus(event)
	assert.Equal("Warning", status)

	event.Check.Status = 2
	status = messageStatus(event)
	assert.Equal("Critical", status)
}

func TestContainerStyle(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	event.Check.Status = 0
	assert.Equal("good", containerStyle(event))

	event.Check.Status = 1
	assert.Equal("warning", containerStyle(event))

	event.Check.Status = 2
	assert.Equal("attention", containerStyle(event))
}

func TestBuildAdaptiveCard(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")
	event.Check.Output = "disk is full"
	event.Check.Status = 2

	config.teamsDescriptionTemplate = "{{ .Check.Output }}"
	payload := buildAdaptiveCard(event)

	assert.Equal("message", payload.Type)
	assert.Len(payload.Attachments, 1)
	assert.Equal("application/vnd.microsoft.card.adaptive", payload.Attachments[0].ContentType)
	assert.Equal("AdaptiveCard", payload.Attachments[0].Content.Type)
	assert.Equal("1.4", payload.Attachments[0].Content.Version)

	// Verify it marshals to valid JSON
	jsonData, err := json.Marshal(payload)
	assert.NoError(err)
	assert.NotEmpty(jsonData)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	var apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		// Verify it's valid JSON
		var payload AdaptiveCardPayload
		err := json.Unmarshal(body, &payload)
		assert.NoError(err)

		// Verify structure
		assert.Equal("message", payload.Type)
		assert.Len(payload.Attachments, 1)
		assert.Equal("application/vnd.microsoft.card.adaptive", payload.Attachments[0].ContentType)
		assert.Equal("AdaptiveCard", payload.Attachments[0].Content.Type)

		assert.Equal("application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write([]byte(`{"status": "accepted"}`))
		require.NoError(t, err)
	}))
	defer apiStub.Close()

	config.teamsWebhookURL = apiStub.URL
	config.teamsDescriptionTemplate = "{{ .Check.Output }}"
	err := sendMessage(event)
	assert.NoError(err)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	var apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer apiStub.Close()

	config.teamsWebhookURL = apiStub.URL
	config.teamsDescriptionTemplate = "{{ .Check.Output }}"
	err := sendMessage(event)
	assert.Error(err)
	assert.Contains(err.Error(), "HTTP 400")
}

func TestCheckArgs(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")

	// Test with env var set
	_ = os.Setenv("TEAMS_WEBHOOK", "http://example.com/webhook")
	config.teamsWebhookURL = os.Getenv("TEAMS_WEBHOOK")
	assert.NoError(checkArgs(event))

	// Test missing webhook
	config.teamsWebhookURL = ""
	_ = os.Unsetenv("TEAMS_WEBHOOK")
	assert.Error(checkArgs(event))
}
