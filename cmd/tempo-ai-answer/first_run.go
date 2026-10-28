package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func (a *app) bindIO(cmd *cobra.Command) {
	input := cmd.InOrStdin()
	a.reader = bufio.NewReader(input)
	a.writer = cmd.OutOrStdout()
	a.inputTTY = nil
	if file, ok := input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		a.inputTTY = file
	}
}

func (a *app) loadConfig() (config.Config, error) {
	if a.inputTTY == nil {
		return config.Load(a.configPath)
	}
	if _, err := os.Stat(a.configPath); err == nil {
		return config.Load(a.configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, err
	}

	cfg, err := promptFirstRunConfig(a.reader, a.output(), func(prompt string) (string, error) {
		return readSecret(a.inputTTY, a.output(), prompt)
	})
	if err != nil {
		return config.Config{}, err
	}
	if err := config.CreateNew(a.configPath, cfg); err != nil {
		if errors.Is(err, os.ErrExist) {
			return config.Load(a.configPath)
		}
		return config.Config{}, fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Fprintf(a.output(), "\n[已保存] 配置已写入：%s\n正在继续启动。\n", a.configPath)
	return cfg, nil
}

func readSecret(input *os.File, output io.Writer, prompt string) (string, error) {
	fmt.Fprint(output, prompt)
	value, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(output)
	if err != nil {
		return "", fmt.Errorf("读取隐藏输入失败: %w", err)
	}
	return string(value), nil
}

func promptFirstRunConfig(reader *bufio.Reader, output io.Writer, readSecret func(string) (string, error)) (config.Config, error) {
	cfg := config.Defaults()
	fmt.Fprintln(output, "首次启动配置")
	fmt.Fprintln(output, "平台密码和 API 密钥输入时不会显示；配置信息只保存在本机配置文件中。")

	var err error
	if cfg.Platform.BaseURL, err = promptValue(reader, output, "Tempo 平台地址（含 /tempo-talents/ 路径）: ", ""); err != nil {
		return config.Config{}, err
	}
	if cfg.Platform.Username, err = promptValue(reader, output, "平台账号: ", ""); err != nil {
		return config.Config{}, err
	}
	if cfg.Platform.Password, err = readSecret("平台密码（输入不回显）: "); err != nil {
		return config.Config{}, err
	}
	if strings.TrimSpace(cfg.Platform.Password) == "" {
		return config.Config{}, errors.New("平台密码不能为空；配置未保存，请重新运行程序")
	}

	fmt.Fprintln(output, "AI 服务：1) DeepSeek 网页登录（默认）  2) OpenAI 兼容 API")
	provider, err := promptValue(reader, output, "选择 [1]: ", "1")
	if err != nil {
		return config.Config{}, err
	}
	switch provider {
	case "1":
		cfg.AI.Provider = "deepseek_web"
	case "2":
		cfg.AI.Provider = "openai_compatible"
		if cfg.AI.BaseURL, err = promptValue(reader, output, "API 地址 [https://api.deepseek.com]: ", "https://api.deepseek.com"); err != nil {
			return config.Config{}, err
		}
		if cfg.AI.Model, err = promptValue(reader, output, "模型 [deepseek-chat]: ", "deepseek-chat"); err != nil {
			return config.Config{}, err
		}
		key, keyErr := readSecret("API 密钥（留空使用 TEMPO_AI_API_KEY）: ")
		if keyErr != nil {
			return config.Config{}, keyErr
		}
		cfg.AI.APIKey = strings.TrimSpace(key)
	default:
		return config.Config{}, errors.New("AI 服务选择无效；配置未保存，请重新运行程序")
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, fmt.Errorf("配置未保存：%w；请核对后重新运行程序", err)
	}
	return cfg, nil
}

func promptValue(reader *bufio.Reader, output io.Writer, prompt, defaultValue string) (string, error) {
	for {
		fmt.Fprint(output, prompt)
		line, err := reader.ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
			return "", fmt.Errorf("读取配置输入失败: %w", err)
		}
		value := strings.TrimSpace(line)
		if value == "" {
			value = defaultValue
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(output, "此项不能为空，请重新输入。")
	}
}
