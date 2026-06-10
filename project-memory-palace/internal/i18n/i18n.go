package i18n

import "sync"

var (
	mu          sync.RWMutex
	currentLang = "en"
)

var translations = map[string]map[string]string{
	"en": {
		"app_title": "Project Memory Palace", "browse": "Browse", "search": "Search",
		"search_placeholder": "Search memories...", "recent": "Recent",
		"search_results": "Search Results", "open_project": "Open Project...",
		"exit": "Exit", "mcp_running": "Running", "mcp_stopped": "Stopped",
		"mcp_start": "Start MCP Server", "mcp_stop": "Stop MCP Server",
		"status": "Status", "type": "Type", "title": "Title", "updated": "Updated",
		"select_memory": "Select a memory to view details", "summary": "Summary",
		"content": "Content", "source": "Source", "tags": "Tags", "scope": "Scope",
		"relations": "Relations", "confidence": "Confidence", "id_label": "ID",
		"empty": "(empty)", "none": "(none)", "loading": "Loading...",
		"ready": "Ready", "error": "Error", "items": "items", "memories": "memories",
		"results_for": "results for", "copied": "Copied", "updated_to": "Updated",
		"copy_id": "Copy ID", "mark_as": "Mark as", "show": "Show",
		"rebuilding": "Rebuilding index...", "rebuilt": "Index rebuilt",
		"searching": "Searching...", "project_label": "Project Root",
		"detail_summary": "Summary", "detail_content": "Content",
		"detail_source": "Source", "detail_tags": "Tags", "detail_scope": "Scope",
		"detail_relations": "Relations", "mcp_start_short": "Start MCP",
		"mcp_stop_short": "Stop MCP", "mcp_na": "MCP N/A",
	},
	"zh": {
		"app_title": "项目记忆宫殿", "browse": "浏览", "search": "搜索",
		"search_placeholder": "搜索记忆...", "recent": "最近",
		"search_results": "搜索结果", "open_project": "打开项目...",
		"exit": "退出", "mcp_running": "运行中", "mcp_stopped": "已停止",
		"mcp_start": "启动 MCP 服务", "mcp_stop": "停止 MCP 服务",
		"status": "状态", "type": "类型", "title": "标题", "updated": "更新时间",
		"select_memory": "选择一条记忆查看详情", "summary": "摘要", "content": "内容",
		"source": "来源", "tags": "标签", "scope": "范围", "relations": "关联",
		"confidence": "置信度", "id_label": "编号", "empty": "（空）", "none": "（无）",
		"loading": "加载中...", "ready": "就绪", "error": "错误", "items": "条",
		"memories": "条记忆", "results_for": "的搜索结果", "copied": "已复制",
		"updated_to": "已更新", "copy_id": "复制 ID", "mark_as": "标记为",
		"show": "显示", "rebuilding": "正在重建索引...", "rebuilt": "索引已重建",
		"searching": "搜索中...", "project_label": "项目目录",
		"detail_summary": "摘要", "detail_content": "内容", "detail_source": "来源",
		"detail_tags": "标签", "detail_scope": "范围", "detail_relations": "关联",
		"mcp_start_short": "启动 MCP", "mcp_stop_short": "停止 MCP", "mcp_na": "MCP 不可用",
	},
}

func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := translations[currentLang]; ok {
		if v, ok := m[key]; ok { return v }
	}
	if m, ok := translations["en"]; ok {
		if v, ok := m[key]; ok { return v }
	}
	return key
}

func SetLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := translations[lang]; ok { currentLang = lang }
}

func GetLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}
