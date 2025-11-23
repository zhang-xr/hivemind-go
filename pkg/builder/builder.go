package builder

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"hivemind-go/pkg/agent"
	"hivemind-go/pkg/llmclient"
	"hivemind-go/pkg/tools"
)

type ToolConfig interface{}

type AssistantConstructor func(ctx *tools.Context, client *llmclient.LLMClient, cfg *AgentConfig) tools.Tool

type AssistantConfig struct {
	Constructor AssistantConstructor

	SubAgentConfig *AgentConfig
}

type AgentConfig struct {
	Name          string
	SystemPrompt  string
	MaxIterations int

	Tools []ToolConfig
}

func setupLogger(agentName string, ctx *tools.Context) (*log.Logger, error) {

	sanitizedAgentName := regexp.MustCompile(`[<>:"/\\|?*\s.]`).ReplaceAllString(agentName, "_")
	var logDir string

	if parentLogDir, ok := ctx.GetString("agent_log_dir"); ok {

		subagentsDir := filepath.Join(parentLogDir, "subagents")
		logDir = filepath.Join(subagentsDir, sanitizedAgentName+"_logs")
	} else {

		logDir = filepath.Join("output", sanitizedAgentName+"_logs")
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建日志目录 %s: %w", logDir, err)
	}

	ctx.Set("agent_log_dir", logDir, false)

	logFilePath := filepath.Join(logDir, "messages.log")

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("无法打开日志文件 %s: %w", logFilePath, err)
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)

	logger := log.New(multiWriter, fmt.Sprintf("[%s] ", agentName), log.LstdFlags)
	return logger, nil
}

func BuildAgent(config *AgentConfig, llmClient *llmclient.LLMClient, baseCtx *tools.Context) (*agent.Agent, error) {
	logger, err := setupLogger(config.Name, baseCtx)
	if err != nil {
		return nil, fmt.Errorf("设置 logger 失败: %w", err)
	}

	var agentTools []tools.Tool

	for _, tConf := range config.Tools {

		switch v := tConf.(type) {

		case tools.Tool:
			agentTools = append(agentTools, v)

		case reflect.Type:

			instance, ok := reflect.New(v).Interface().(tools.Tool)
			if !ok {
				return nil, fmt.Errorf("类型 %v 没有实现 tools.Tool 接口", v)
			}

			if sc, ok := instance.(interface{ SetContext(*tools.Context) }); ok {
				sc.SetContext(baseCtx)
			}
			agentTools = append(agentTools, instance)

		case AssistantConfig:
			assistant, err := buildAssistant(v, llmClient, baseCtx)
			if err != nil {
				return nil, fmt.Errorf("构建 assistant 工具失败: %w", err)
			}
			agentTools = append(agentTools, assistant)

		default:
			return nil, fmt.Errorf("不支持的工具配置类型: %T", tConf)
		}
	}

	agentInstance := agent.NewAgent(
		config.Name,
		llmClient,
		agent.WithSystemPrompt(config.SystemPrompt),
		agent.WithMaxIterations(config.MaxIterations),
		agent.WithTools(agentTools...),
		agent.WithLogger(logger),
	)

	return agentInstance, nil
}

func buildAssistant(config AssistantConfig, llmClient *llmclient.LLMClient, baseCtx *tools.Context) (tools.Tool, error) {
	if config.Constructor == nil {
		return nil, fmt.Errorf("assistant config 缺少构造函数")
	}

	assistant := config.Constructor(baseCtx, llmClient, config.SubAgentConfig)
	return assistant, nil
}
