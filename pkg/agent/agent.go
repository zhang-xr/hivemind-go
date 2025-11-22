package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"agenthive-go/pkg/history"
	"agenthive-go/pkg/llmclient"
	"agenthive-go/pkg/tools"
	"agenthive-go/pkg/types"
)

type Agent struct {
	name            string
	llmClient       *llmclient.LLMClient
	jsonOutputLLM   *JSONOutputLLM
	tools           map[string]tools.Tool
	systemPrompt    string
	maxIterations   int
	historyStrategy history.Strategy
	messages        []types.Message
	logger          *log.Logger

	mu sync.Mutex

	backgroundJobs map[string]*Job

	jobsMu sync.Mutex
}

type Job struct {
	ID        string
	ToolName  string
	ToolInput map[string]interface{}

	ResultChan chan string
	ErrChan    chan error
	Ctx        context.Context
	CancelFunc context.CancelFunc
}

type AgentOption func(*Agent)

func WithTools(agentTools ...tools.Tool) AgentOption {
	return func(a *Agent) {
		a.tools = make(map[string]tools.Tool)
		for _, t := range agentTools {
			a.tools[t.Name()] = t
		}
	}
}

func WithSystemPrompt(prompt string) AgentOption {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

func WithMaxIterations(n int) AgentOption {
	return func(a *Agent) {
		a.maxIterations = n
	}
}

func WithHistoryStrategy(strategy history.Strategy) AgentOption {
	return func(a *Agent) {
		a.historyStrategy = strategy
	}
}

func WithLogger(logger *log.Logger) AgentOption {
	return func(a *Agent) {
		a.logger = logger
	}
}

func NewAgent(name string, llmClient *llmclient.LLMClient, opts ...AgentOption) *Agent {

	a := &Agent{
		name:            name,
		llmClient:       llmClient,
		jsonOutputLLM:   NewJSONOutputLLM(llmClient),
		systemPrompt:    "你是一个有用的 AI 助手。",
		maxIterations:   25,
		historyStrategy: &history.NoOpStrategy{},
		messages:        []types.Message{},
		logger:          log.Default(),
		backgroundJobs:  make(map[string]*Job),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

func (a *Agent) addMessage(role, content, msgType string) {

	a.mu.Lock()

	defer a.mu.Unlock()
	a.messages = append(a.messages, types.Message{
		Role:    role,
		Content: content,
		Type:    msgType,
	})
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	a.logger.Printf("Agent '%s' 开始运行。初始输入: %s", a.name, userInput)

	fullSystemPrompt, err := a.jsonOutputLLM.buildSystemPrompt(a.systemPrompt, a.tools)
	if err != nil {
		return "", fmt.Errorf("failed to build system prompt: %w", err)
	}
	a.addMessage("system", fullSystemPrompt, "system_prompt")
	a.addMessage("user", userInput, "user_input")

	isWaitingForJobs := false
	iterationCount := 0

	for iterationCount < a.maxIterations {
		a.logger.Printf("\n----- [Agent: %s, 迭代: %d/%d] -----\n", a.name, iterationCount+1, a.maxIterations)

		if injectedResult := a.checkAndInjectBackgroundJobs(); injectedResult {
			a.logger.Println("检测到后台任务完成，注入结果。")
			isWaitingForJobs = false
		}

		if isWaitingForJobs {
			if len(a.backgroundJobs) > 0 {
				a.logger.Printf("正在等待 %d 个后台任务完成...", len(a.backgroundJobs))
				time.Sleep(1 * time.Second)
				continue
			} else {
				a.logger.Println("所有后台任务已完成。")
				a.addMessage("user", "所有后台任务已完成，请总结结果。", "system_note")
				isWaitingForJobs = false
			}
		}

		iterationCount++

		a.mu.Lock()
		managedHistory := a.historyStrategy.Apply(a.messages)
		a.mu.Unlock()

		llmMsgs := make([]llmclient.Message, len(managedHistory))
		for i, m := range managedHistory {
			llmMsgs[i] = llmclient.Message{Role: m.Role, Content: m.Content}
		}

		a.logger.Println("正在调用 LLM...")
		llmResponse, err := a.llmClient.Invoke(ctx, llmMsgs, 3)
		if err != nil {
			a.logger.Printf("LLM 调用错误: %v", err)
			return "", fmt.Errorf("iteration %d: failed to get LLM response: %w", iterationCount, err)
		}
		a.logger.Printf("LLM 原始响应: %s", llmResponse.Content)
		a.addMessage("assistant", llmResponse.Content, "llm_output")

		action, err := a.jsonOutputLLM.parseLLMResponse(llmResponse.Content)
		if err != nil {

			errorMsg := fmt.Sprintf("解析 LLM 响应失败: %v. 将此错误告知 LLM 并重试。", err)
			a.logger.Printf("解析错误: %v", err)
			a.addMessage("user", errorMsg, "parse_error")
			continue
		}
		a.logger.Printf("解析出的动作: Action=%s, Status=%s", action.Action, action.Status)

		if action.Status == "complete" || (action.Action == "finish" && action.Status != "continue") {
			if len(a.backgroundJobs) > 0 {
				a.logger.Println("Agent 想要结束，但仍有后台任务在运行。进入等待模式。")
				a.addMessage("user", "系统提示: 你的完成请求已收到，但后台任务仍在运行。系统将等待它们完成后再生成最终摘要。要明确等待而不结束，请使用 'wait' 动作。", "system_note")
				isWaitingForJobs = true
				continue
			}
			a.logger.Println("检测到 '完成' 状态。正在结束执行。")
			finalResponse, _ := action.ActionInput["final_response"].(string)
			a.logger.Printf("最终响应: %s", finalResponse)
			return finalResponse, nil
		}

		if action.Action == "wait" {
			if len(a.backgroundJobs) > 0 {
				a.logger.Println("动作是 'wait'，且有后台任务在运行。进入等待模式。")
				isWaitingForJobs = true
				continue
			} else {
				a.logger.Println("警告: Agent 选择 'wait' 动作，但没有正在运行的后台任务。")
				a.addMessage("user", "警告: 你使用了 'wait' 动作，但没有正在运行的后台任务。请选择另一个动作或使用 'finish' 完成任务。", "system_warning")
				continue
			}
		}

		if tool, ok := a.tools[action.Action]; ok {

			if runInBackground, _ := action.ActionInput["run_in_background"].(bool); runInBackground {
				a.startBackgroundTask(ctx, tool, action.Action, action.ActionInput)
				continue
			}
			a.logger.Printf("正在使用参数执行工具 '%s': %v", action.Action, action.ActionInput)

			toolResult, err := a.executeTool(ctx, tool, action.ActionInput)

			if err != nil {
				toolResult = fmt.Sprintf("工具 '%s' 执行失败: %v", action.Action, err)
				a.logger.Printf("工具执行错误: %s", toolResult)
			} else {
				a.logger.Printf("工具 '%s' 执行结果: %s", action.Action, toolResult)
			}
			a.addMessage("user", toolResult, "tool_result")
		} else {

			errorMsg := fmt.Sprintf("错误: 工具 '%s' 不存在。可用工具: %s", action.Action, strings.Join(a.getToolNames(), ", "))
			a.logger.Println(errorMsg)
			a.addMessage("user", errorMsg, "tool_error")
		}
	}

	a.logger.Printf("已达到最大迭代次数 (%d)，但未找到答案。", a.maxIterations)
	return "已达到最大迭代次数，但未找到答案。", nil
}

func (a *Agent) startBackgroundTask(ctx context.Context, tool tools.Tool, toolName string, args map[string]interface{}) {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()

	jobID := uuid.New().String()
	a.logger.Printf("启动后台任务 '%s' (ID: %s)", toolName, jobID)

	jobCtx, cancel := context.WithCancel(ctx)

	job := &Job{
		ID:         jobID,
		ToolName:   toolName,
		ToolInput:  args,
		ResultChan: make(chan string, 1),
		ErrChan:    make(chan error, 1),
		Ctx:        jobCtx,
		CancelFunc: cancel,
	}
	a.backgroundJobs[jobID] = job

	go func() {
		defer close(job.ResultChan)
		defer close(job.ErrChan)

		res, err := tool.Execute(job.Ctx, args)
		if err != nil {
			job.ErrChan <- err
			return
		}
		job.ResultChan <- res
	}()

	startMsg := fmt.Sprintf("后台任务已启动。任务名称: '%s', 任务ID: '%s'。你可以继续执行其他操作，稍后会自动收到结果。", toolName, jobID)
	a.addMessage("user", startMsg, "tool_result")
}

func (a *Agent) checkAndInjectBackgroundJobs() bool {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()

	injectedResult := false
	for jobID, job := range a.backgroundJobs {
		select {
		case result := <-job.ResultChan:

			msg := fmt.Sprintf("后台任务 '%s' (%s) 已完成。\n参数: %v\n结果:\n%s", job.ToolName, job.ID, job.ToolInput, result)
			a.logger.Printf("注入后台任务结果: %s", msg)
			a.addMessage("user", msg, "background_tool_result")
			delete(a.backgroundJobs, jobID)
			injectedResult = true
		case err := <-job.ErrChan:

			msg := fmt.Sprintf("后台任务 '%s' (%s) 失败。\n参数: %v\n错误: %v", job.ToolName, job.ID, job.ToolInput, err)
			a.logger.Printf("注入后台任务错误: %s", msg)
			a.addMessage("user", msg, "background_tool_error")
			delete(a.backgroundJobs, jobID)
			injectedResult = true
		default:

		}
	}
	return injectedResult
}

func (a *Agent) executeTool(ctx context.Context, tool tools.Tool, args map[string]interface{}) (string, error) {

	toolCtx, cancel := context.WithTimeout(ctx, 300*time.Second)

	defer cancel()

	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		res, err := tool.Execute(toolCtx, args)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- res
	}()

	select {
	case res := <-resultChan:

		return res, nil
	case err := <-errChan:

		return "", err
	case <-toolCtx.Done():

		return "", fmt.Errorf("工具执行超时: %w", toolCtx.Err())
	}
}

func (a *Agent) getToolNames() []string {

	names := make([]string, 0, len(a.tools))
	for name := range a.tools {
		names = append(names, name)
	}
	return names
}
