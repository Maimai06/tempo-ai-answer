package main

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

func TestPromptFirstRunConfigDeepSeek(t *testing.T) {
	t.Setenv("TEMPO_AI_API_KEY", "")
	var output strings.Builder
	reader := bufio.NewReader(strings.NewReader("http://10.0.40.37:32080/tempo-talents/#/login\nstudent-01\n\n"))
	cfg, err := promptFirstRunConfig(reader, &output, func(prompt string) (string, error) {
		fmt.Fprint(&output, prompt)
		return "platform-password", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Provider != "deepseek_web" || cfg.Platform.BaseURL != "http://10.0.40.37:32080/tempo-talents/" || cfg.Platform.Username != "student-01" || cfg.Platform.Password != "platform-password" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if strings.Contains(output.String(), "platform-password") {
		t.Fatal("password was echoed")
	}
}

func TestPromptFirstRunConfigOpenAICompatible(t *testing.T) {
	t.Setenv("TEMPO_AI_API_KEY", "")
	var output strings.Builder
	reader := bufio.NewReader(strings.NewReader("https://tempo.example/tempo-talents/\nstudent-02\n2\nhttps://api.example/v1\nfixture-model\n"))
	secrets := []string{"platform-password", "api-secret"}
	cfg, err := promptFirstRunConfig(reader, &output, func(prompt string) (string, error) {
		fmt.Fprint(&output, prompt)
		secret := secrets[0]
		secrets = secrets[1:]
		return secret, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Provider != "openai_compatible" || cfg.AI.BaseURL != "https://api.example/v1" || cfg.AI.Model != "fixture-model" || cfg.AI.APIKey != "api-secret" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if strings.Contains(output.String(), "platform-password") || strings.Contains(output.String(), "api-secret") {
		t.Fatal("secret input was echoed")
	}
}

func TestPromptFirstRunConfigRejectsMissingAPIKey(t *testing.T) {
	t.Setenv("TEMPO_AI_API_KEY", "")
	reader := bufio.NewReader(strings.NewReader("https://tempo.example/tempo-talents/\nstudent-03\n2\nhttps://api.example/v1\nfixture-model\n"))
	secrets := []string{"platform-password", ""}
	_, err := promptFirstRunConfig(reader, &strings.Builder{}, func(string) (string, error) {
		secret := secrets[0]
		secrets = secrets[1:]
		return secret, nil
	})
	if err == nil || !strings.Contains(err.Error(), "API 密钥") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestPromptFirstRunConfigUsesAPIKeyEnvironment(t *testing.T) {
	t.Setenv("TEMPO_AI_API_KEY", "environment-secret")
	var output strings.Builder
	reader := bufio.NewReader(strings.NewReader("https://tempo.example/tempo-talents/\nstudent-04\n2\n\n\n"))
	secrets := []string{"platform-password", ""}
	cfg, err := promptFirstRunConfig(reader, &output, func(prompt string) (string, error) {
		fmt.Fprint(&output, prompt)
		secret := secrets[0]
		secrets = secrets[1:]
		return secret, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.APIKey != "" || cfg.AI.APIKeyEnv != "TEMPO_AI_API_KEY" {
		t.Fatalf("API key was not left in the environment: %+v", cfg.AI)
	}
	if strings.Contains(output.String(), "environment-secret") {
		t.Fatal("environment API key was echoed")
	}
}
