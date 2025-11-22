package assistants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"agenthive-go/pkg/builder"
	"agenthive-go/pkg/llmclient"
	"agenthive-go/pkg/tools"
)

type TaskDelegator struct {
	baseCtx        *tools.Context
	llmClient      *llmclient.LLMClient
	subAgentConfig *builder.AgentConfig
}

func NewTaskDelegator(ctx *tools.Context, client *llmclient.LLMClient, cfg *builder.AgentConfig) tools.Tool {
	return &TaskDelegator{
		baseCtx:        ctx,
		llmClient:      client,
		subAgentConfig: cfg,
	}
}

func (t *TaskDelegator) Name() string {
	return "TaskDelegator"
}

func (t *TaskDelegator) Description() string {
	return `任务委托器 - 用于将一个子任务委托给子代理进行处理。
在你需要将复杂问题分解成更小、更易于管理的部分时使用它。`
}

func (t *TaskDelegator) Parameters() json.RawMessage {

	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "要委托给子代理的任务的清晰、简洁的描述。"
			},
			"run_in_background": {
				"type": "boolean",
				"description": "如果为 true, 则在后台运行工具, 不阻塞主流程。"
			}
		},
		"required": ["task"]
	}`)
}

func (t *TaskDelegator) Execute(ctx context.Context, args map[string]interface{}) (string, error) {

	taskDesc, ok := args["task"].(string)
	if !ok {
		return "", fmt.Errorf("无效的参数：'task' 必须是一个字符串")
	}

	subAgentCtx, err := t.baseCtx.Copy()
	if err != nil {
		return "", fmt.Errorf("创建子 agent 上下文失败: %w", err)
	}

	subAgent, err := builder.BuildAgent(t.subAgentConfig, t.llmClient, subAgentCtx)
	if err != nil {

		return "", fmt.Errorf("构建子 agent 失败: %w", err)
	}

	result, err := subAgent.Run(ctx, taskDesc)
	if err != nil {
		return "", fmt.Errorf("子 agent 执行失败: %w", err)
	}

	return result, nil
}

type ParallelTaskDelegator struct {
	baseCtx        *tools.Context
	llmClient      *llmclient.LLMClient
	subAgentConfig *builder.AgentConfig
}

func NewParallelTaskDelegator(ctx *tools.Context, client *llmclient.LLMClient, cfg *builder.AgentConfig) tools.Tool {
	return &ParallelTaskDelegator{
		baseCtx:        ctx,
		llmClient:      client,
		subAgentConfig: cfg,
	}
}

func (p *ParallelTaskDelegator) Name() string {
	return "ParallelTaskDelegator"
}

func (p *ParallelTaskDelegator) Description() string {
	return `并行任务委托器 - 用于将多个独立的子任务分发给并行执行的子代理。
在子任务之间没有严格的执行顺序依赖时使用它，可以提高效率。`
}

func (p *ParallelTaskDelegator) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tasks": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "需要并行执行的独立子任务描述的列表。"
			},
			"run_in_background": {
				"type": "boolean",
				"description": "如果为 true, 则在后台运行工具, 不阻塞主流程。"
			}
		},
		"required": ["tasks"]
	}`)
}

func (p *ParallelTaskDelegator) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	tasks, ok := args["tasks"].([]interface{})
	if !ok {
		return "", fmt.Errorf("无效的参数：'tasks' 必须是一个字符串列表")
	}

	var wg sync.WaitGroup

	results := make([]string, len(tasks))
	errs := make([]error, len(tasks))

	for i, task := range tasks {
		taskDesc, ok := task.(string)
		if !ok {
			results[i] = fmt.Sprintf("任务 #%d 的输入无效（不是字符串）", i)
			continue
		}

		wg.Add(1)

		go func(index int, description string) {

			defer wg.Done()

			subAgentCtx, err := p.baseCtx.Copy()
			if err != nil {
				errs[index] = fmt.Errorf("任务 #%d: 创建上下文失败: %w", index, err)
				return
			}

			parallelSubAgentConfig := *p.subAgentConfig
			parallelSubAgentConfig.Name = fmt.Sprintf("%s_task_%d", p.subAgentConfig.Name, index)

			subAgent, err := builder.BuildAgent(&parallelSubAgentConfig, p.llmClient, subAgentCtx)
			if err != nil {
				errs[index] = fmt.Errorf("任务 #%d: 构建 agent 失败: %w", index, err)
				return
			}

			result, err := subAgent.Run(ctx, description)
			if err != nil {
				errs[index] = fmt.Errorf("任务 #%d: 执行失败: %w", index, err)
				results[index] = fmt.Sprintf("子 agent 执行失败: %v", err)
				return
			}

			results[index] = result
			errs[index] = nil

		}(i, taskDesc)

	}

	wg.Wait()

	var finalResult strings.Builder
	var aggregatedErrs []string
	for i := range tasks {
		finalResult.WriteString(fmt.Sprintf("--- 任务 #%d 结果 ---\n", i+1))
		if errs[i] != nil {
			errStr := fmt.Sprintf("错误: %v", errs[i])
			finalResult.WriteString(errStr + "\n")
			aggregatedErrs = append(aggregatedErrs, errStr)
		} else {
			finalResult.WriteString(results[i] + "\n")
		}
	}

	if len(aggregatedErrs) > 0 {

		return finalResult.String(), errors.New(strings.Join(aggregatedErrs, "; "))
	}

	return finalResult.String(), nil
}
