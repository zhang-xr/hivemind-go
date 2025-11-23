package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"hivemind-go/pkg/llmclient"
	"hivemind-go/pkg/tools"
)

type LLMResponseAction struct {
	Action string `json:"action"`

	ActionInput map[string]interface{} `json:"action_input"`

	Thought string `json:"thought"`

	Status string `json:"status"`
}

type JSONOutputLLM struct {
	llmClient *llmclient.LLMClient
}

func NewJSONOutputLLM(client *llmclient.LLMClient) *JSONOutputLLM {
	return &JSONOutputLLM{
		llmClient: client,
	}
}

func (j *JSONOutputLLM) buildSystemPrompt(systemPrompt string, toolMap map[string]tools.Tool) (string, error) {

	var toolSectionBuilder strings.Builder
	for _, tool := range toolMap {

		formattedTool, err := tools.FormatForPrompt(tool)
		if err != nil {
			return "", err
		}
		toolSectionBuilder.WriteString(formattedTool)
		toolSectionBuilder.WriteString("\n\n")
	}

	formatSection := `{
    "type": "object",
    "properties": {
        "thought": {
            "type": "string",
            "description": "在这里逐步思考。分析当前情况、目标、可用工具和对话历史。决定是调用工具还是使用 'finish' 动作来提供最终答案。"
        },
        "action": {
            "type": "string",
            "description": "选择下一个动作。必须是可用的工具名称之一, 'wait', 或 'finish'。"
        },
        "action_input": {
            "type": "object",
            "description": "工具调用的参数或最终响应。如果 action 是工具名称，请提供该工具所需的参数；如果是 'finish'，请使用 'final_response' 作为此处的键来提供最终响应。"
        },
        "status": {
            "type": "string",
            "enum": ["continue", "complete"],
            "description": "如果选择了工具，则必须是 'continue'；如果选择了 'finish'，则必须是 'complete'。"
        }
    },
    "required": ["thought", "action", "action_input", "status"]
}`

	finalPrompt := fmt.Sprintf(
		`%s

--- 可用工具 ---
%s
--- 后台执行与任务调度 ---
- 在工具参数中设置 "run_in_background": true 来异步运行任务。
- 后台任务的结果将在其完成后自动注入。
- 当你需要等待后台结果才能继续时，请使用 'wait' 动作。

--- 响应格式要求 ---
你必须严格以 JSON 格式响应。不要在 JSON 对象之外添加任何其他文本。输出一个单一的 JSON 对象，该对象必须符合以下 JSON 模式:
%s`,
		systemPrompt,
		toolSectionBuilder.String(),
		formatSection,
	)

	return finalPrompt, nil
}

func (j *JSONOutputLLM) parseLLMResponse(responseText string) (*LLMResponseAction, error) {

	start := strings.Index(responseText, "{")
	end := strings.LastIndex(responseText, "}")
	if start == -1 || end == -1 || start > end {
		return nil, fmt.Errorf("无法在响应中找到有效的 JSON 对象")
	}
	jsonStr := responseText[start : end+1]

	var parsedResponse LLMResponseAction

	err := json.Unmarshal([]byte(jsonStr), &parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("解析 LLM 响应 JSON 失败: %w", err)
	}

	return &parsedResponse, nil
}
