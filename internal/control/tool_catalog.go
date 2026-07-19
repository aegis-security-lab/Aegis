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
	catalog := map[string]ToolSnapshot{
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
			Description: "把当前 Issue 原子拆分为 2–8 个持久化子 Issues，并把执行权交还给调度器；评论唤醒已完成 Issue 后调用会自动重新打开父 Issue。",
			Parameters: []ToolParameterSnapshot{
				parameter("requestKey", "string", "当前父 Issue 内唯一且稳定的幂等键。", true),
				parameter("summary", "string", "为什么需要拆分的简短说明。", true),
				{
					Name: "children", Type: "array<object>", Required: true,
					Description: "需要创建的 2–8 个可独立验证的子 Issues。",
					Children: []ToolParameterSnapshot{
						parameter("title", "string", "子 Issue 标题，最多 120 个字符。", true),
						parameter("description", "string", "子 Issue 的执行范围与上下文。", true),
						parameter("objective", "string", "验收 Agent 用于判断是否完成的明确目标。", true),
						{Name: "priority", Type: "enum", Description: "子 Issue 优先级。", Required: true, Enum: []string{"critical", "high", "medium", "low"}},
						parameter("agentId", "string", "负责该子 Issue 的启用 Agent ID；空字符串表示由调度器选择。", true),
						parameter("dependsOn", "array<number>", "依赖的较早子项序号，使用从 1 开始的索引。", true),
					},
				},
			},
		},
		"aegis_create_task": {
			Name: "aegis_create_task", Label: "Create Aegis task", Source: "aegis_extension",
			Description: "根据当前管家对话创建一个真实的顶层 Aegis 任务，并立即交给现有调度器选择或启动负责 Agent。",
			Parameters: []ToolParameterSnapshot{
				parameter("title", "string", "清晰具体的任务标题，最多 120 个字符。", true),
				parameter("taskDescription", "string", "执行背景、范围、约束和需要完成的工作。", true),
				parameter("objective", "string", "可选的可验证目标；为空时任务不进入验收流程。", false),
				{Name: "priority", Type: "enum", Description: "任务优先级。", Required: true, Enum: []string{"critical", "high", "medium", "low"}},
				{Name: "workMode", Type: "enum", Description: "自主执行或需要引导审批。", Required: true, Enum: []string{"autonomous", "guided"}},
				parameter("agentId", "string", "明确适合时指定启用的 Agent ID；空字符串表示由调度器选择。", false),
				parameter("workspace", "string", "可选工作目录；为空时使用全局工作区。", false),
			},
		},
		"aegis_publish_attachment": {
			Name: "aegis_publish_attachment", Label: "Publish attachment", Source: "aegis_extension",
			Description: "把 Issue 工作目录中的交付文件复制为持久化附件，并在 Execution 完成时挂载到 Agent 评论。",
			Parameters: []ToolParameterSnapshot{
				parameter("path", "string", "已生成文件的绝对路径或工作目录相对路径。", true),
				parameter("name", "string", "可选的附件下载文件名。", false),
				parameter("attachmentDescription", "string", "可选的交付物简短说明。", false),
			},
		},
		"aegis_report_progress": {
			Name: "aegis_report_progress", Label: "Report work progress", Source: "aegis_extension",
			Description: "记录当前 Session 的阶段性工作进度，包括刚完成的阶段、阶段结果以及接下来正在进行的工作。",
			Parameters: []ToolParameterSnapshot{
				parameter("stage", "string", "刚完成的阶段名称，最多 120 个字符。", true),
				parameter("summary", "string", "这个阶段完成、变更或确认的内容及证据，最多 4000 个字符。", true),
				parameter("currentActivity", "string", "现在开始进行的具体工作，最多 1000 个字符。", true),
			},
		},
		"aegis_broadcast": {
			Name: "aegis_broadcast", Label: "Broadcast task information", Source: "aegis_extension",
			Description: "把经过验证且对其他工作有价值的关键信息持久化，并广播给同一顶层任务树中所有正在执行的其他 Agent。",
			Parameters: []ToolParameterSnapshot{
				parameter("subject", "string", "广播主题，最多 160 个字符。", true),
				parameter("message", "string", "包含发现、证据、影响范围和协作价值的 Markdown 消息，最多 6000 个字符。", true),
				{Name: "importance", Type: "enum", Description: "广播重要性。", Required: true, Enum: []string{"normal", "important", "critical"}},
			},
		},
		"aegis_list_broadcasts": {
			Name: "aegis_list_broadcasts", Label: "List task broadcasts", Source: "aegis_extension",
			Description: "只读获取同一顶层任务树内最近的广播历史，按时间从新到旧返回。",
			Parameters: []ToolParameterSnapshot{
				parameter("limit", "number", "最多返回的广播数量，范围 1–50，默认 20。", false),
			},
		},
		"aegis_list_validation_attachments": {
			Name: "aegis_list_validation_attachments", Label: "List validation attachments", Source: "aegis_extension",
			Description: "只读列出当前验收所对应 Worker Execution 发布的附件，不能访问其他 Issue 或 Execution 的附件。",
			Parameters:  []ToolParameterSnapshot{},
		},
		"aegis_read_validation_attachment": {
			Name: "aegis_read_validation_attachment", Label: "Read validation attachment", Source: "aegis_extension",
			Description: "按字节分段读取当前验收范围内的文本附件，用于核对附件交付物是否满足 Issue 目标。",
			Parameters: []ToolParameterSnapshot{
				parameter("attachmentId", "string", "附件清单中的附件 ID。", true),
				parameter("offset", "number", "继续读取的字节偏移，默认 0。", false),
				parameter("limit", "number", "本次最多读取的字节数，范围 1–32768。", false),
			},
		},
		"aegis_get_memo": {
			Name: "aegis_get_memo", Label: "Read Agent memo", Source: "aegis_extension",
			Description: "读取当前 Agent 的持久化备忘录。备忘录用于跨会话保留稳定的用户或 Leader 偏好、长期工作倾向、反复纠错和经常犯错的提醒。",
			Parameters:  []ToolParameterSnapshot{},
		},
		"aegis_update_memo": {
			Name: "aegis_update_memo", Label: "Update Agent memo", Source: "aegis_extension",
			Description: "替换当前 Agent 的持久化备忘录。用户或 Leader 明确要求记住的内容、稳定个人习惯、长期工作偏好、经常犯错或反复被纠正的地方应写入；不要记录一次性任务细节、秘密、个人敏感信息或未经验证的推断。更新前先读取并保留仍有效的条目。",
			Parameters: []ToolParameterSnapshot{
				parameter("content", "string", "完整的新备忘录内容，最多 20000 个字符；此操作会替换旧内容。", true),
			},
		},
		"aegis_request_rework": {
			Name: "aegis_request_rework", Label: "Request Issue rework", Source: "aegis_extension",
			Description: "为已完成或待复核的当前 Issue 请求重新打开、细化并执行。仅当用户或 Leader 明确要求补做、重新拆解或重新执行时使用；请求会按返工审批策略进入审批中心，批准后创建新的工作 Execution 并重新获得 checkout。不要用它代替普通解释或一次性评论回复。",
			Parameters: []ToolParameterSnapshot{
				parameter("reason", "string", "为什么现有结果需要返工，以及触发请求的用户或 Leader 要求。", true),
				parameter("requestedOutcome", "string", "批准后应完成的具体结果、建议拆解范围和验收依据。", true),
			},
		},
		"aegis_uncover_search": {
			Name: "aegis_uncover_search", Label: "Search cyberspace engines", Source: "aegis_extension",
			Description: "使用 ProjectDiscovery uncover 对一个指定网络空间引擎执行原生语法检索，返回归一化预览，并把完整结果按所选格式发布为 Issue 附件。仅限已授权、非破坏性的信息收集。",
			Parameters: []ToolParameterSnapshot{
				{Name: "engine", Type: "enum", Description: "只选择一个搜索引擎；query 必须使用该引擎自己的语法。", Required: true, Enum: []string{"shodan", "censys", "fofa", "shodan-idb", "quake", "hunter", "zoomeye", "netlas", "criminalip", "publicwww", "hunterhow", "google", "odin", "binaryedge", "onyphe", "driftnet", "greynoise", "daydaymap", "nerdydata"}},
				parameter("query", "string", "原样传给所选引擎的查询语法；不要混用其他引擎的字段和运算符。", true),
				parameter("limit", "number", "去重后的最大结果数，范围 1–1000，默认 100。", false),
				{Name: "format", Type: "enum", Description: "完整结果的附件导出格式，默认 jsonl。", Required: false, Enum: []string{"txt", "json", "jsonl", "csv"}},
				{Name: "field", Type: "enum", Description: "TXT 导出时每行输出的字段，默认 ip:port。", Required: false, Enum: []string{"ip:port", "host:port", "ip", "host", "port", "url"}},
				parameter("timeout", "number", "搜索超时秒数，范围 5–120，默认 30。", false),
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
	purpose := parameter("description", "string", "用一句简短的话说明本次调用工具的目的和预期获得的结果。", true)
	for name, definition := range catalog {
		definition.Parameters = append([]ToolParameterSnapshot{purpose}, definition.Parameters...)
		catalog[name] = definition
	}
	return catalog
}
