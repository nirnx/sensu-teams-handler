package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sensu-community/sensu-plugin-sdk/sensu"
	"github.com/sensu-community/sensu-plugin-sdk/templates"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
)

// HandlerConfig contains the Teams handler configuration
type HandlerConfig struct {
	sensu.PluginConfig
	teamsWebhookURL          string
	teamsDescriptionTemplate string
	teamsAlertCritical       bool
}

const (
	webHookURL          = "webhook-url"
	descriptionTemplate = "description-template"
	alertCritical       = "alert-on-critical"

	defaultTemplate      = "{{ .Check.Output }}"
	defaultAlert    bool = false
)

var (
	config = HandlerConfig{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-teams-handler",
			Short:    "The Sensu Go Microsoft Teams handler for notifying a channel",
			Keyspace: "sensu.io/plugins/teams/config",
		},
	}

	teamsConfigOptions = []*sensu.PluginConfigOption{
		{
			Path:      webHookURL,
			Env:       "TEAMS_WEBHOOK",
			Argument:  webHookURL,
			Shorthand: "w",
			Secret:    true,
			Usage:     "The Teams Workflow webhook URL to send messages to",
			Value:     &config.teamsWebhookURL,
		},
		{
			Path:      descriptionTemplate,
			Env:       "TEAMS_DESCRIPTION_TEMPLATE",
			Argument:  descriptionTemplate,
			Shorthand: "t",
			Default:   defaultTemplate,
			Usage:     "The Teams notification output template, in Golang text/template format",
			Value:     &config.teamsDescriptionTemplate,
		},
		{
			Path:      alertCritical,
			Env:       "TEAMS_ALERT_ON_CRITICAL",
			Argument:  alertCritical,
			Shorthand: "a",
			Default:   defaultAlert,
			Usage:     "Mark the notification as urgent/attention when critical",
			Value:     &config.teamsAlertCritical,
		},
	}
)

// Adaptive Card payload structures for Power Automate Workflow webhooks
type AdaptiveCardPayload struct {
	Type        string       `json:"type"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ContentType string       `json:"contentType"`
	ContentURL  *string      `json:"contentUrl"`
	Content     AdaptiveCard `json:"content"`
}

type AdaptiveCard struct {
	Type    string        `json:"type"`
	Schema  string        `json:"$schema"`
	Version string        `json:"version"`
	Body    []interface{} `json:"body"`
}

type TextBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Size      string `json:"size,omitempty"`
	Weight    string `json:"weight,omitempty"`
	Color     string `json:"color,omitempty"`
	Wrap      bool   `json:"wrap,omitempty"`
	Separator bool   `json:"separator,omitempty"`
	Spacing   string `json:"spacing,omitempty"`
}

type FactSet struct {
	Type  string `json:"type"`
	Facts []Fact `json:"facts"`
}

type Fact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type Container struct {
	Type    string        `json:"type"`
	Style   string        `json:"style,omitempty"`
	Bleed   bool          `json:"bleed,omitempty"`
	Items   []interface{} `json:"items"`
	Spacing string        `json:"spacing,omitempty"`
}

func main() {
	goHandler := sensu.NewGoHandler(&config.PluginConfig, teamsConfigOptions, checkArgs, sendMessage)
	goHandler.Execute()
}

func checkArgs(_ *corev2.Event) error {
	if webhook := os.Getenv("TEAMS_WEBHOOK"); webhook != "" && config.teamsWebhookURL == "" {
		config.teamsWebhookURL = webhook
	}

	if len(config.teamsWebhookURL) == 0 {
		return fmt.Errorf("--%s or TEAMS_WEBHOOK environment variable is required", webHookURL)
	}

	return nil
}

func formattedEventAction(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "RESOLVED"
	default:
		return "ALERT"
	}
}

func chomp(s string) string {
	return strings.Trim(strings.Trim(strings.Trim(s, "\n"), "\r"), "\r\n")
}

func eventKey(event *corev2.Event) string {
	return fmt.Sprintf("%s/%s", event.Entity.Name, event.Check.Name)
}

func eventSummary(event *corev2.Event, maxLength int) string {
	output := chomp(event.Check.Output)
	if len(event.Check.Output) > maxLength {
		output = output[0:maxLength] + "..."
	}
	return fmt.Sprintf("%s:%s", eventKey(event), output)
}

func formattedMessage(event *corev2.Event) string {
	return fmt.Sprintf("%s - %s", formattedEventAction(event), eventSummary(event, 100))
}

func messageColor(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "good"
	case 2:
		return "attention"
	default:
		return "warning"
	}
}

func messageStatus(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "Resolved"
	case 2:
		if config.teamsAlertCritical {
			return "CRITICAL - Attention Required"
		}
		return "Critical"
	default:
		return "Warning"
	}
}

func containerStyle(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "good"
	case 2:
		return "attention"
	default:
		return "warning"
	}
}

func buildAdaptiveCard(event *corev2.Event) AdaptiveCardPayload {
	description, err := templates.EvalTemplate("description", config.teamsDescriptionTemplate, event)
	if err != nil {
		fmt.Printf("%s: Error processing template: %s", config.PluginConfig.Name, err)
	}
	description = strings.Replace(description, `\n`, "\n", -1)

	headerContainer := Container{
		Type:  "Container",
		Style: containerStyle(event),
		Bleed: true,
		Items: []interface{}{
			TextBlock{
				Type:   "TextBlock",
				Text:   fmt.Sprintf("Sensu %s", formattedEventAction(event)),
				Size:   "large",
				Weight: "bolder",
				Color:  "default",
			},
		},
	}

	statusBlock := TextBlock{
		Type:   "TextBlock",
		Text:   fmt.Sprintf("Status: **%s**", messageStatus(event)),
		Size:   "medium",
		Weight: "bolder",
		Wrap:   true,
	}

	facts := FactSet{
		Type: "FactSet",
		Facts: []Fact{
			{Title: "Entity", Value: event.Entity.Name},
			{Title: "Check", Value: event.Check.Name},
			{Title: "Namespace", Value: event.Entity.Namespace},
			{Title: "Occurrences", Value: fmt.Sprintf("%d", event.Check.Occurrences)},
		},
	}

	outputBlock := TextBlock{
		Type:      "TextBlock",
		Text:      "**Output:**",
		Wrap:      true,
		Separator: true,
		Spacing:   "medium",
	}

	descriptionBlock := TextBlock{
		Type: "TextBlock",
		Text: description,
		Wrap: true,
	}

	card := AdaptiveCard{
		Type:    "AdaptiveCard",
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Version: "1.4",
		Body: []interface{}{
			headerContainer,
			statusBlock,
			facts,
			outputBlock,
			descriptionBlock,
		},
	}

	return AdaptiveCardPayload{
		Type: "message",
		Attachments: []Attachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     card,
			},
		},
	}
}

func sendMessage(event *corev2.Event) error {
	payload := buildAdaptiveCard(event)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Teams payload: %v", err)
	}

	resp, err := http.Post(config.teamsWebhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to send Teams message: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Teams webhook returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Notification sent to Microsoft Teams (HTTP %d)\n", resp.StatusCode)
	return nil
}
