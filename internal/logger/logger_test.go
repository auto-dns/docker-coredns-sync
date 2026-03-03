package logger

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/rs/zerolog"
)

func TestSetupLogger_ValidLevels(t *testing.T) {
	levels := []struct {
		input    string
		expected zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"TRACE", zerolog.TraceLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"INFO", zerolog.InfoLevel},
	}

	for _, tc := range levels {
		t.Run(tc.input, func(t *testing.T) {
			var buf bytes.Buffer
			oldStdout := stdout
			stdout = &buf
			defer func() { stdout = oldStdout }()

			cfg := &config.LoggingConfig{Level: tc.input}
			_ = SetupLogger(cfg)

			if zerolog.GlobalLevel() != tc.expected {
				t.Errorf("expected level %v, got %v", tc.expected, zerolog.GlobalLevel())
			}
		})
	}
}

func TestSetupLogger_InvalidLevel_FallbackToInfo(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	cfg := &config.LoggingConfig{Level: "invalid-level"}
	_ = SetupLogger(cfg)

	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("expected fallback to InfoLevel, got %v", zerolog.GlobalLevel())
	}
}

func TestSetupLogger_EmptyLevel_UsesNoLevel(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	cfg := &config.LoggingConfig{Level: ""}
	_ = SetupLogger(cfg)

	// zerolog.ParseLevel("") returns NoLevel without error
	// so SetGlobalLevel is called with NoLevel (which allows all levels)
	if zerolog.GlobalLevel() != zerolog.NoLevel {
		t.Errorf("expected NoLevel for empty level string, got %v", zerolog.GlobalLevel())
	}
}

func TestSetupLogger_HostnameIncluded(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	oldGetHostname := getHostname
	getHostname = func() (string, error) {
		return "test-hostname", nil
	}
	defer func() { getHostname = oldGetHostname }()

	cfg := &config.LoggingConfig{Level: "info"}
	logger := SetupLogger(cfg)

	logger.Info().Msg("test message")

	output := buf.String()
	if !strings.Contains(output, "test-hostname") {
		t.Errorf("expected output to contain hostname 'test-hostname', got: %s", output)
	}
}

func TestSetupLogger_HostnameFallback(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	oldGetHostname := getHostname
	getHostname = func() (string, error) {
		return "", errors.New("hostname unavailable")
	}
	defer func() { getHostname = oldGetHostname }()

	cfg := &config.LoggingConfig{Level: "info"}
	logger := SetupLogger(cfg)

	logger.Info().Msg("test message")

	output := buf.String()
	if !strings.Contains(output, "unknown-host") {
		t.Errorf("expected output to contain 'unknown-host' on hostname error, got: %s", output)
	}
}

func TestSetupLogger_OutputStructure(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	oldGetHostname := getHostname
	getHostname = func() (string, error) {
		return "struct-host", nil
	}
	defer func() { getHostname = oldGetHostname }()

	cfg := &config.LoggingConfig{Level: "info"}
	logger := SetupLogger(cfg)

	logger.Info().Msg("structured output test")

	output := buf.String()

	if !strings.Contains(output, "docker_coredns_sync") {
		t.Errorf("expected output to contain service name 'docker_coredns_sync', got: %s", output)
	}
	if !strings.Contains(output, "struct-host") {
		t.Errorf("expected output to contain host 'struct-host', got: %s", output)
	}
	if !strings.Contains(output, "structured output test") {
		t.Errorf("expected output to contain message, got: %s", output)
	}
}

func TestSetupLogger_WritesToConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := stdout
	stdout = &buf
	defer func() { stdout = oldStdout }()

	cfg := &config.LoggingConfig{Level: "info"}
	logger := SetupLogger(cfg)

	logger.Info().Msg("output test")

	if buf.Len() == 0 {
		t.Error("expected logger to write to configured output buffer")
	}
}
