# Hivemind Go

简洁的 Go 版智能体(Agent)示例与微型框架，采用 ReAct(思考-行动-观察)循环，包含：
- 可插拔的工具接口与示例工具 `FileTool`
- 任务委托助手（串行与并行）
- 简单的历史管理与 JSON 结构化输出解析

## 目录结构
- `cmd/myagent`: 可执行入口与示例
- `pkg/agent`: ReAct 主循环与后台任务等
- `pkg/assistants`: 任务委托工具
- `pkg/builder`: 通过配置构建 Agent
- `pkg/history`: 历史策略
- `pkg/llmclient`: LLM 客户端封装
- `pkg/tools`: 工具接口与上下文
- `pkg/types`: 基础类型

## 快速开始
1. 确保已安装 Go 1.23+。
2. 准备配置文件 `config.toml`（包含所需 LLM 的 `api_key` 与 `base_url` 等）。
3. 运行示例：
   ```bash
   go run ./cmd/myagent
   ```

## 注意
- 为安全起见，建议不要将含有真实密钥的 `config.toml` 推送到公共仓库。
  如需开源，建议：
  1) 将真实配置重命名为 `config.local.toml` 并加入 `.gitignore`
  2) 新增一个 `config.example.toml`（使用占位符）供参考。
- 本仓库的 `go.mod` 模块名为 `agenthive-go`，与远程仓库名不同并不影响运行。
  如需对外引用，建议将模块名改为 `github.com/zhang-xr/hivemind-go` 并全局替换导入路径。

