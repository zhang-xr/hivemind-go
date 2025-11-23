package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"

	"hivemind-go/pkg/assistants"
	"hivemind-go/pkg/builder"
	"hivemind-go/pkg/llmclient"
	"hivemind-go/pkg/tools"
)

type FileTool struct {
	ctx *tools.Context
}

func (f *FileTool) Name() string { return "FileTool" }
func (f *FileTool) Description() string {
	return "一个可以读写文件的工具。在写入前，请务必先思考一下要写入什么内容。"
}
func (f *FileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {"type": "string", "enum": ["read", "write"]},
			"path": {"type": "string"},
			"content": {"type": "string", "description": "写入文件时需要"},
			"run_in_background": {"type": "boolean", "description": "如果为 true, 则在后台运行工具, 不阻塞主流程。"}
		},
		"required": ["operation", "path"]
	}`)
}
func (f *FileTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	op, _ := args["operation"].(string)
	path, _ := args["path"].(string)

	time.Sleep(1 * time.Second)

	switch op {
	case "read":

		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("成功读取文件 '%s' 的内容: %s", path, string(content)), nil
	case "write":
		content, ok := args["content"].(string)
		if !ok {
			return "", fmt.Errorf("写入操作需要 'content' 参数")
		}

		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("成功将内容写入文件 '%s'。", path), nil
	}
	return "", fmt.Errorf("不支持的操作: %s", op)
}

func (f *FileTool) SetContext(ctx *tools.Context) { f.ctx = ctx }

func findProjectRoot() (string, error) {

	_, b, _, _ := runtime.Caller(0)

	dir := filepath.Dir(b)

	for i := 0; i < 5; i++ {

		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("无法找到项目根目录 (go.mod)")
}

func main() {

	projectRoot, err := findProjectRoot()
	if err != nil {

		panic(err)
	}
	configPath := filepath.Join(projectRoot, "config.toml")

	config, err := llmclient.LoadConfig(configPath)
	if err != nil {
		panic(fmt.Sprintf("无法加载配置: %v", err))
	}
	llmClient := llmclient.NewLLMClient(config)
	baseCtx := tools.NewContext()

	fileAgentConfig := &builder.AgentConfig{
		Name:          "FileOperatorAgent",
		SystemPrompt:  "你是一个专门操作文件的助手。使用 FileTool 来读取或写入文件。",
		MaxIterations: 5,
		Tools: []builder.ToolConfig{

			reflect.TypeOf(FileTool{}),
		},
	}

	taskDelegatorConfig := builder.AssistantConfig{

		Constructor: func(ctx *tools.Context, client *llmclient.LLMClient, cfg *builder.AgentConfig) tools.Tool {
			return assistants.NewTaskDelegator(ctx, client, cfg)
		},
		SubAgentConfig: fileAgentConfig,
	}

	parallelDelegatorConfig := builder.AssistantConfig{
		Constructor: func(ctx *tools.Context, client *llmclient.LLMClient, cfg *builder.AgentConfig) tools.Tool {
			return assistants.NewParallelTaskDelegator(ctx, client, cfg)
		},
		SubAgentConfig: fileAgentConfig,
	}

	managerAgentConfig := &builder.AgentConfig{
		Name:          "ManagerAgent",
		SystemPrompt:  "你是一个主管 agent。你的工作是分析用户请求，并使用你可用的工具来完成它。对于单个、连续的任务，使用 TaskDelegator。对于多个可以并行完成的独立任务，使用 ParallelTaskDelegator。",
		MaxIterations: 5,
		Tools:         []builder.ToolConfig{taskDelegatorConfig, parallelDelegatorConfig},
	}

	managerAgent, err := builder.BuildAgent(managerAgentConfig, llmClient, baseCtx)
	if err != nil {
		panic(fmt.Sprintf("无法构建主管 agent: %v", err))
	}

	fmt.Println("\n============================")
	fmt.Println("=== 示例 1: 串行任务委托 ===")
	fmt.Println("============================")

	userInputSerial := "请帮我将 '你好世界' 这段文字写入 'greeting.txt' 文件。"

	result, err := managerAgent.Run(context.Background(), userInputSerial)
	if err != nil {
		fmt.Printf("Agent 执行出错: %v\n", err)
	} else {
		fmt.Printf("\n最终结果: %s\n", result)
	}

	fmt.Println("\n\n===============================")
	fmt.Println("=== 示例 2: 并行任务委托 ===")
	fmt.Println("===============================")

	managerAgent2, _ := builder.BuildAgent(managerAgentConfig, llmClient, tools.NewContext())

	userInputParallel := "请并行执行以下任务：1. 将 '第一个文件' 写入 'file1.txt'。 2. 将 '第二个文件' 写入 'file2.txt'。"
	result, err = managerAgent2.Run(context.Background(), userInputParallel)
	if err != nil {
		fmt.Printf("Agent 执行出错: %v\n", err)
	} else {
		fmt.Printf("\n最终结果: %s\n", result)
	}

	fmt.Println("\n\n====================================")
	fmt.Println("=== 示例 3: 异步后台任务委托 ===")
	fmt.Println("====================================")
	managerAgent3, _ := builder.BuildAgent(managerAgentConfig, llmClient, tools.NewContext())
	userInputAsync := "请在后台执行以下任务: 1. 将 '后台文件一' 写入 'bg_file1.txt'。 2. 将 '后台文件二' 写入 'bg_file2.txt'。在这两个任务运行时，请立刻读取 'greeting.txt' 文件的内容。最后，等待所有后台任务完成后，告诉我所有任务都已成功。"
	result, err = managerAgent3.Run(context.Background(), userInputAsync)
	if err != nil {
		fmt.Printf("Agent 执行出错: %v\n", err)
	} else {
		fmt.Printf("\n最终结果: %s\n", result)
	}
}
