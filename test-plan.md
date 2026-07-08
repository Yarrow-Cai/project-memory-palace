# pmem 修复验证测试计划

## 测试环境
- 项目路径: C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace\
- 二进制: bin\pmem.exe

## 测试步骤

### 1. 编译验证
```
cd C:\Users\Atop\Desktop\pmem\project-memory-palace\project-memory-palace
go build -o bin/pmem.exe ./cmd/pmem
go vet ./...
```
→ 必须无错误

### 2. MCP stdio 测试（验证 B2: 错误码区分）
启动 pmem，用 stdin 发送 JSON-RPC 请求：

```bash
# 测试 invalid params 错误码
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"remember","arguments":{"memory":{"type":"invalid_type","title":"","summary":"","content":""}}}}' | timeout 3 ./bin/pmem.exe serve-mcp . 2>/dev/null | head -1
```
→ 应该返回 -32602 (Invalid params)，不是 -32603

```bash
# 测试 unknown tool 错误码
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nonexistent_tool","arguments":{}}}}' | timeout 3 ./bin/pmem.exe serve-mcp . 2>/dev/null | head -1
```
→ 应该返回 -32601 (Method not found)

### 3. SSE 测试（验证 B1: sendEvent overload）
启动 Web 服务器，模拟大量并发请求测试 channel 满的情况：
```bash
./bin/pmem.exe serve-web . &
sleep 2
# 发送 100 个快速请求
for i in $(seq 1 100); do curl -s http://127.0.0.1:8147/api/count & done
wait
```
→ 服务器不应该崩溃，日志中可能有 WARNING "channel full" 但不应丢弃响应

### 4. Audit CLI 测试（验证 B3: 审计检测扩展）
```bash
# 确保有 test 卡片
./bin/pmem.exe audit . 2>&1
```
→ 应该正确标记 high_confidence_inference

### 5. WebUI 回归测试
```bash
curl -s http://127.0.0.1:8147/api/recent?limit=5
curl -s http://127.0.0.1:8147/api/search?q=test
curl -s http://127.0.0.1:8147/api/hot?limit=5
```
→ 所有端点正常返回

### 6. 清理
```bash
kill $(pgrep pmem.exe) 2>/dev/null
```

## 验收标准
1. ✅ 编译通过，go vet 无警告
2. ✅ tools/call 参数错误返回 -32602
3. ✅ tools/call 未知工具返回 -32601
4. ✅ SSE 服务器不崩溃，channel full 时客户端收到错误
5. ✅ audit 正确处理所有 source kind
6. ✅ 所有 Web API 端点正常响应
