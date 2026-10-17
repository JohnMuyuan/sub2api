# 账号请求时区

在创建、编辑非 OpenAI 账号时，可选择：

- **保留原请求**：默认选项，保持原有行为。
- **自动（按实际出口 IP）**：在 HTTP 转发已选定账号代理或区域路由后，使用原项目的出口探测器解析该出口的 IANA 时区。出口变化会使用不同的缓存项；成功缓存一分钟，失败短暂缓存并保留原请求时区。
- **指定时区**：例如 `Asia/Taipei`，直接使用选择值。

配置保存于账号 `extra.request_timezone`：留空或删除为保留，`auto` 为自动，其余为时区目录中的 IANA 名称。创建和编辑接口校验该值。无需数据库迁移。

## 生效范围

共用 HTTP 上游转发层在出站前改写受支持的 JSON POST 请求。支持 Anthropic Messages、Chat Completions、Responses、Gemini 原生以及 Antigravity 的嵌套 Gemini 请求中的完整 `<environment_context>` 文本块；只修改其中已有的 `<timezone>` 值。

同时支持 `web_search` / `web_search_*` 工具已有的 `user_location.timezone`，以及最终出站请求中已有的 `X-Timezone` / `X-Client-Timezone` 请求头。Anthropic 对搜索工具时区的定义见[官方文档](https://platform.claude.com/docs/zh-CN/agents-and-tools/tool-use/web-search-tool)。

请求未携带可识别的时区字段时，设置不会产生额外字段。普通聊天文字、工具参数及结果、图片、推理块和历史日期保持原样。AWS 签名请求、带内容摘要或压缩的请求、WebSocket 和绕过共用 HTTP 上游层的传输不适用。设置不改变机器的系统时区或网络出口。

OpenAI/Codex 继续使用原有账号配置和系统开关，其 HTTP、WebSocket 和按出口 IP 自动绑定行为保持原有规则。其他平台的账号选择独立生效。

## 验证

```sh
cd backend
go test ./internal/service -run 'Test(AccountRequestTimezone|RequestTimezone|OpenAIRequest|RewriteOpenAIRequestEnvironment)' -count=1
go test ./internal/repository -run 'TestHTTPUpstream.*Timezone|Test.*Regional' -count=1
go run github.com/google/wire/cmd/wire check ./cmd/server
go build ./cmd/server
```

```sh
cd frontend
pnpm exec vitest run src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/OpenAIRequestTimezoneField.spec.ts
pnpm run build
```
