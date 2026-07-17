package control

func snapshotTools(names []string) []ToolSnapshot {
	catalog := toolCatalog()
	result := make([]ToolSnapshot, 0, len(names))
	for _, name := range uniqueStrings(names) {
		if definition, ok := catalog[name]; ok {
			result = append(result, definition)
			continue
		}
		result = append(result, ToolSnapshot{
			Name: name, Label: name, Source: "unknown", Parameters: []ToolParameterSnapshot{},
			Description: "该工具由 Pi 运行时或扩展提供，但 Aegis 没有可序列化的定义元数据。",
		})
	}
	return result
}

func toolCatalog() map[string]ToolSnapshot {
	parameter := func(name, kind, description string, required bool) ToolParameterSnapshot {
		return ToolParameterSnapshot{Name: name, Type: kind, Description: description, Required: required}
	}
	return map[string]ToolSnapshot{
		"read": {
			Name: "read", Label: "Read", Source: "pi_builtin",
			Description: "读取文本文件或受支持的图片。文本可按行偏移和数量分段读取；图片会作为多模态附件返回给模型。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "要读取的文件路径，可以是相对路径或绝对路径。", true),
				parameter("offset", "number", "开始读取的行号，从 1 开始。", false),
				parameter("limit", "number", "最多读取的行数。", false),
			},
		},
		"write": {
			Name: "write", Label: "Write", Source: "pi_builtin",
			Description: "把完整内容写入文件。文件不存在时创建，存在时覆盖，并自动创建父目录。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "要写入的文件路径，可以是相对路径或绝对路径。", true),
				parameter("content", "string", "写入文件的完整文本内容。", true),
			},
		},
		"edit": {
			Name: "edit", Label: "Edit", Source: "pi_builtin",
			Description: "使用精确文本替换编辑单个文件。每个 oldText 必须在原文件中唯一匹配，多个替换按原始内容并行定位。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "要编辑的文件路径，可以是相对路径或绝对路径。", true),
				{
					Name: "edits", Type: "array<object>", Required: true,
					Description: "一个或多个互不重叠的精确文本替换。",
					Children: []ToolParameterSnapshot{
						parameter("oldText", "string", "需要被替换的原始文本；必须精确且唯一匹配。", true),
						parameter("newText", "string", "用于替换 oldText 的新文本。", true),
					},
				},
			},
		},
		"bash": {
			Name: "bash", Label: "Bash", Source: "pi_builtin",
			Description: "在当前 Issue 工作目录中执行 Bash 命令并返回标准输出和标准错误；可配置超时时间。",
			Parameters: []ToolParameterSnapshot{
				parameter("command", "string", "需要执行的 Bash 命令。", true),
				parameter("timeout", "number", "可选超时时间，单位为秒；不填写时没有默认超时。", false),
			},
		},
		"grep": {
			Name: "grep", Label: "Grep", Source: "pi_builtin",
			Description: "搜索文件内容并返回匹配行、文件路径与行号，遵循 .gitignore。支持正则、字面量、文件 Glob 和上下文行。",
			Parameters: []ToolParameterSnapshot{
				parameter("pattern", "string", "正则表达式或字面量搜索内容。", true),
				parameter("path", "string", "要搜索的目录或文件；默认当前目录。", false),
				parameter("glob", "string", "文件过滤 Glob，例如 '*.ts' 或 '**/*.spec.ts'。", false),
				parameter("ignoreCase", "boolean", "是否忽略大小写；默认 false。", false),
				parameter("literal", "boolean", "是否把 pattern 当作普通字符串而非正则；默认 false。", false),
				parameter("context", "number", "每个匹配项前后显示的上下文行数；默认 0。", false),
				parameter("limit", "number", "最多返回的匹配数量；默认 100。", false),
			},
		},
		"find": {
			Name: "find", Label: "Find", Source: "pi_builtin",
			Description: "按 Glob 搜索文件，返回相对于搜索目录的匹配路径并遵循 .gitignore。",
			Parameters: []ToolParameterSnapshot{
				parameter("pattern", "string", "文件匹配 Glob，例如 '*.ts'、'**/*.json'。", true),
				parameter("path", "string", "搜索目录；默认当前目录。", false),
				parameter("limit", "number", "最多返回的结果数量；默认 1000。", false),
			},
		},
		"ls": {
			Name: "ls", Label: "List directory", Source: "pi_builtin",
			Description: "按字母顺序列出目录内容，包含隐藏文件，并为目录添加 '/' 后缀。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "需要列出的目录；默认当前目录。", false),
				parameter("limit", "number", "最多返回的条目数；默认 500。", false),
			},
		},
		"aegis_create_subissues": {
			Name: "aegis_create_subissues", Label: "Create child Issues", Source: "aegis_extension",
			Description: "把当前已签出的 Issue 原子拆分为 2–8 个持久化子 Issues，并把执行权交还给调度器。",
			Parameters: []ToolParameterSnapshot{
				parameter("requestKey", "string", "当前父 Issue 内唯一且稳定的幂等键。", true),
				parameter("summary", "string", "为什么需要拆分的简短说明。", true),
				{
					Name: "children", Type: "array<object>", Required: true,
					Description: "需要创建的 2–8 个可独立验证的子 Issues。",
					Children: []ToolParameterSnapshot{
						parameter("title", "string", "子 Issue 标题，最多 120 个字符。", true),
						parameter("description", "string", "子 Issue 的执行范围与上下文。", true),
						parameter("acceptanceCriteria", "string", "可观察、可验证的完成标准。", true),
						{Name: "priority", Type: "enum", Description: "子 Issue 优先级。", Required: true, Enum: []string{"critical", "high", "medium", "low"}},
						parameter("agentId", "string", "负责该子 Issue 的启用 Agent ID；空字符串表示由调度器选择。", true),
						parameter("dependsOn", "array<number>", "依赖的较早子项序号，使用从 1 开始的索引。", true),
					},
				},
			},
		},
		"aegis_publish_attachment": {
			Name: "aegis_publish_attachment", Label: "Publish attachment", Source: "aegis_extension",
			Description: "把 Issue 工作目录中的交付文件复制为持久化附件，并在 Execution 完成时挂载到 Agent 评论。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "已生成文件的绝对路径或工作目录相对路径。", true),
				parameter("name", "string", "可选的附件下载文件名。", false),
				parameter("description", "string", "可选的交付物简短说明。", false),
			},
		},
		"aegis_search_knowledge": {
			Name: "aegis_search_knowledge", Label: "Search knowledge", Source: "aegis_extension",
			Description: "在当前 Agent 关联的知识库中进行只读检索。系统先做关键词召回，再由内置只读检索 Agent 对 Markdown 文档开头进行排序和摘要。",
			Parameters: []ToolParameterSnapshot{
				parameter("query", "string", "需要查找的问题、概念或关键词。", true),
				parameter("knowledgeBaseId", "string", "可选的已关联知识库 ID；不填写时检索全部关联知识库。", false),
				parameter("limit", "number", "返回结果上限，范围 1–10，默认 5。", false),
			},
		},
	}
}
