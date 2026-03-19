# Sensu Teams Handler

A [Sensu Go](https://docs.sensu.io/sensu-go/latest/) handler that sends alerts to Microsoft Teams channels via [Power Automate Workflow webhooks](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook) using Adaptive Cards.

## Overview

This handler reads Sensu events from stdin and posts formatted Adaptive Card notifications to a Microsoft Teams channel. It supports the newer Power Automate Workflow webhook type (replacement for the deprecated Office 365 Connectors).

The notification card includes:
- Color-coded header indicating event status (resolved/warning/critical)
- Entity name, check name, namespace, and occurrence count
- Check output with configurable Go templates

## Installation

### From source

```bash
git clone https://github.com/nirnx/sensu-teams-handler.git
cd sensu-teams-handler
go build -o sensu-teams-handler
```

### Static build (Linux amd64)

```bash
bash build.sh
```

### Sensu Asset (recommended)

The GitHub release archives are built in Sensu asset format (binary under `bin/`), so they work directly as runtime assets.

**Using `sensuctl`:**

```bash
# Register the asset directly from a GitHub release URL
sensuctl asset create sensu-teams-handler \
  --url "https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_linux_amd64.tar.gz" \
  --sha512 "$(curl -sL https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_sha512-checksums.txt | grep linux_amd64 | awk '{print $1}')"
```

Or for multiple platforms, create an asset definition YAML:

```yaml
type: Asset
api_version: core/v2
metadata:
  name: sensu-teams-handler
  namespace: default
spec:
  builds:
    - url: https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_linux_amd64.tar.gz
      sha512: <sha512sum>
      filters:
        - entity.system.os == 'linux'
        - entity.system.arch == 'amd64'
    - url: https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_linux_arm64.tar.gz
      sha512: <sha512sum>
      filters:
        - entity.system.os == 'linux'
        - entity.system.arch == 'arm64'
```

```bash
sensuctl create -f sensu-teams-handler-asset.yml
```

**Verify the asset is registered:**

```bash
sensuctl asset list
sensuctl asset info sensu-teams-handler
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TEAMS_WEBHOOK` | Yes | | Power Automate Workflow webhook URL |
| `TEAMS_DESCRIPTION_TEMPLATE` | No | `{{ .Check.Output }}` | Go text/template for the output field |
| `TEAMS_ALERT_ON_CRITICAL` | No | `false` | Mark critical alerts with "Attention Required" |

### Command Line Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--webhook-url` | `-w` | The Teams Workflow webhook URL |
| `--description-template` | `-t` | Go text/template for the output field |
| `--alert-on-critical` | `-a` | Mark critical alerts with "Attention Required" |

### Sensu Handler Definition

```yaml
type: Handler
api_version: core/v2
metadata:
  name: teams
  namespace: default
spec:
  type: pipe
  command: sensu-teams-handler
  env_vars:
    - TEAMS_WEBHOOK=https://your-webhook-url-here
  runtime_assets:
    - sensu-teams-handler
  timeout: 30
  filters:
    - is_incident
    - not_silenced
```

### Annotation Overrides

You can override the webhook URL per check or entity using annotations:

```yaml
type: CheckConfig
api_version: core/v2
metadata:
  name: my-check
  annotations:
    sensu.io/plugins/teams/config/webhook-url: "https://alternate-webhook-url"
```

### Description Template

The description template uses [Go text/template](https://pkg.go.dev/text/template) syntax with the full Sensu event object available. Examples:

```bash
# Default - just check output
--description-template "{{ .Check.Output }}"

# Include entity and check info
--description-template "Entity: {{ .Entity.Name }} Check: {{ .Check.Name }} Output: {{ .Check.Output }}"

# With labels
--description-template "Region: {{ .Entity.Labels.region }} - {{ .Check.Output }}"
```

## Usage

### Test locally

```bash
cat testing/data/example-event.json | \
  TEAMS_WEBHOOK="https://your-webhook-url" \
  ./sensu-teams-handler
```

### Development

```bash
# Run tests
go test -v ./...

# Build
go build -o sensu-teams-handler
```

## Quick Start: Full Sensu Setup

**1. Register the asset:**

```bash
sensuctl asset create sensu-teams-handler \
  --url "https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_linux_amd64.tar.gz" \
  --sha512 "$(curl -sL https://github.com/nirnx/sensu-teams-handler/releases/download/v0.1.0/sensu-teams-handler_0.1.0_sha512-checksums.txt | grep linux_amd64 | awk '{print $1}')"
```

**2. Create the handler:**

```bash
sensuctl handler create teams \
  --type pipe \
  --command "sensu-teams-handler" \
  --runtime-assets sensu-teams-handler \
  --env-vars "TEAMS_WEBHOOK=https://your-webhook-url-here" \
  --timeout 30 \
  --filters is_incident,not_silenced
```

**3. Assign the handler to a check:**

```bash
sensuctl check set-handlers my-check teams
```

Or add it to an existing check definition:

```yaml
type: CheckConfig
api_version: core/v2
metadata:
  name: my-check
  namespace: default
spec:
  command: check-cpu --warning 80 --critical 90
  handlers:
    - teams
  subscriptions:
    - linux
  interval: 60
```

**4. Verify:**

```bash
sensuctl handler info teams
sensuctl event list
```
