# iframe 嵌入安全配置实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`)语法来跟踪进度。

**目标：** 在 GatewayOps 系统配置中增加 iframe 嵌入控制，默认拒绝嵌入；开启后支持允许全部或按自定义域名 Glob 白名单允许嵌入。

**架构：** 后端新增独立 hostname Glob matcher 和 iframe 响应头中间件；现有 `app_settings` 持久化 iframe 配置，并通过 `/api/system-settings` 读写。前端在现有系统配置页新增“安全与嵌入”卡片。高级 Glob 只由 GatewayOps 匹配，最终向浏览器输出精确的 CSP `frame-ancestors`。

**技术栈：** Go 1.23、Gin、GORM/PostgreSQL、React 19、TypeScript、Vite。

---

## 文件结构

- 创建 `backend/internal/security/hostglob.go`：域名规则校验、规范化和匹配，保持纯函数。
- 创建 `backend/internal/security/hostglob_test.go`：覆盖精确匹配、`*`、`**`、段内通配及非法规则。
- 创建 `backend/internal/api/frame_policy.go`：读取系统设置、判定父来源、输出 iframe 安全响应头。
- 创建 `backend/internal/api/frame_policy_test.go`：覆盖关闭、允许全部、白名单命中/拒绝、Referer 缺失等。
- 创建 `backend/internal/storage/settings_test.go`：验证 iframe 设置默认值、保存/读取和 JSON 白名单持久化。
- 修改 `backend/internal/storage/model.go`：扩展现有 `UISettings`，加入 iframe 字段。
- 修改 `backend/internal/storage/settings.go`：增加 iframe setting keys、读取/保存和默认值。
- 修改 `backend/internal/api/system_settings.go`：保存前校验 mode 和每条 Glob 规则，非法配置返回 400。
- 修改 `backend/internal/api/api.go`：在前端页面响应路径应用 iframe policy。
- 修改 `frontend/lib/api-types.ts`：扩展 `SystemSettings` 类型。
- 修改 `frontend/lib/system-settings.ts`：增加默认值。
- 修改 `frontend/app/system-settings-page.tsx`：新增“安全与嵌入”配置 UI。

### 任务 1：实现 hostname Glob matcher

**文件：**
- 创建：`backend/internal/security/hostglob.go`
- 创建：`backend/internal/security/hostglob_test.go`

- [ ] **步骤 1：编写失败测试**

测试表至少包含：

```go
tests := []struct {
    pattern string
    host    string
    want    bool
}{
    {"*", "anything.example", true},
    {"example.com", "example.com", true},
    {"example.com", "a.example.com", false},
    {"*.example.com", "a.example.com", true},
    {"*.example.com", "a.b.example.com", false},
    {"*.example.com", "example.com", false},
    {"**.example.com", "a.example.com", true},
    {"**.example.com", "a.b.example.com", true},
    {"**.example.com", "example.com", false},
    {"a*abc.com", "a123abc.com", true},
    {"a*abc.com", "a.fooabc.com", false},
    {"*.devai.*", "a.devai.com", true},
    {"*.devai.*", "a.b.devai.co.uk", false},
    {"**.devai.**", "a.b.devai.c.d", true},
    {"**.devai.**", "mydevai.com", false},
}
```

同时验证以下非法规则返回错误：

```text
https://example.com
example.com/path
example.com:8443
a..example.com
ab**cd.example.com
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```powershell
cd backend
go test ./internal/security -run HostGlob -count=1
```

预期：FAIL，因为 matcher 尚不存在。

- [ ] **步骤 3：实现最少匹配逻辑**

提供接口：

```go
package security

func ValidateHostPattern(pattern string) error
func MatchHostPattern(pattern, host string) (bool, error)
func MatchAnyHostPattern(patterns []string, host string) (bool, error)
```

实现原则：

- 输入统一 `strings.ToLower(strings.TrimSpace(...))`
- 去掉 hostname 尾部单个 `.`
- 单独 `*` 直接匹配全部
- `*` 只在单个 label 内匹配字符
- 完整 label `**` 匹配一个或多个 label
- `**` 不允许出现在 label 内
- 用 label 递归/DP 匹配，不用用户输入直接拼正则

- [ ] **步骤 4：运行测试确认通过**

```powershell
cd backend
go test ./internal/security -count=1
```

预期：PASS。

### 任务 2：扩展系统设置持久化与 API 校验

**文件：**
- 修改：`backend/internal/storage/model.go`
- 修改：`backend/internal/storage/settings.go`
- 创建：`backend/internal/storage/settings_test.go`
- 修改：`backend/internal/api/system_settings.go`

- [ ] **步骤 1：编写 storage 失败测试**

扩展设置模型：

```go
type UISettings struct {
    // existing fields...
    IframeEmbeddingEnabled   bool     `json:"iframe_embedding_enabled"`
    IframeEmbeddingMode      string   `json:"iframe_embedding_mode"`
    IframeEmbeddingAllowlist []string `json:"iframe_embedding_allowlist"`
}
```

默认值测试要求：

```go
if got.IframeEmbeddingEnabled {
    t.Fatal("iframe embedding must be disabled by default")
}
if got.IframeEmbeddingMode != "allowlist" {
    t.Fatalf("mode = %q", got.IframeEmbeddingMode)
}
if len(got.IframeEmbeddingAllowlist) != 0 {
    t.Fatalf("allowlist = %#v", got.IframeEmbeddingAllowlist)
}
```

保存后重新读取应保持：

```go
UISettings{
    IframeEmbeddingEnabled: true,
    IframeEmbeddingMode: "allowlist",
    IframeEmbeddingAllowlist: []string{"*.example.com", "a*abc.com"},
}
```

- [ ] **步骤 2：运行测试验证失败**

```powershell
cd backend
go test ./internal/storage -run UISettings -count=1
```

- [ ] **步骤 3：实现设置读写**

新增 key：

```go
SettingSecurityIframeEnabled   = "security.iframe.enabled"
SettingSecurityIframeMode      = "security.iframe.mode"
SettingSecurityIframeAllowlist = "security.iframe.allowlist"
```

白名单用 JSON 数组保存；读取不存在时返回默认 `false / allowlist / []`。

- [ ] **步骤 4：API 保存校验**

在 PUT `/system-settings` 保存前：

```go
if settings.IframeEmbeddingMode != "all" && settings.IframeEmbeddingMode != "allowlist" {
    fail(c, http.StatusBadRequest, errors.New("invalid iframe embedding mode"))
    return
}
for _, pattern := range settings.IframeEmbeddingAllowlist {
    if err := security.ValidateHostPattern(pattern); err != nil {
        fail(c, http.StatusBadRequest, fmt.Errorf("invalid iframe allowlist pattern %q: %w", pattern, err))
        return
    }
}
```

mode=all 时允许白名单保留，但不参与运行时判断。

- [ ] **步骤 5：运行相关测试**

```powershell
cd backend
go test ./internal/storage ./internal/api -count=1
```

预期：PASS。

### 任务 3：实现 iframe 响应策略中间件

**文件：**
- 创建：`backend/internal/api/frame_policy.go`
- 创建：`backend/internal/api/frame_policy_test.go`
- 修改：`backend/internal/api/api.go`

- [ ] **步骤 1：编写失败测试**

构造页面请求，验证：

关闭：

```text
Content-Security-Policy: frame-ancestors 'none'
X-Frame-Options: DENY
```

允许全部：

```text
不输出 frame-ancestors
不输出 X-Frame-Options
```

白名单命中，Referer=`https://sub.example.com:8443/page`：

```text
Content-Security-Policy: frame-ancestors https://sub.example.com:8443
```

白名单不命中或 iframe 请求缺少 Referer：

```text
Content-Security-Policy: frame-ancestors 'none'
```

顶层导航即使没有 Referer 也应正常返回页面；限制通过响应头生效，不返回 HTTP 403。

- [ ] **步骤 2：运行测试确认失败**

```powershell
cd backend
go test ./internal/api -run FramePolicy -count=1
```

- [ ] **步骤 3：实现 policy**

提供：

```go
func applyFramePolicy(c *gin.Context, settings *storage.Settings)
```

行为：

- settings 不可用时安全回退到 `frame-ancestors 'none'` + `DENY`
- enabled=false：CSP none + XFO DENY
- mode=all：删除/不设置上述限制头
- mode=allowlist：
  - iframe/frame 请求从 `Referer` 解析 origin
  - hostname 命中任一 pattern 时输出精确 origin
  - 无法判定或未命中则 CSP none
- 非 iframe 顶层请求仍输出当前策略对应 CSP；CSP 不阻止顶层导航

- [ ] **步骤 4：挂到前端页面响应**

在 `registerFrontend` 的 SPA/static 页面响应前应用 policy。不要给 `/api/*`、SSE、`/healthz` 添加 iframe 策略。

必要时调整签名：

```go
func registerFrontend(r *gin.Engine, dist fs.FS, settings *storage.Settings)
```

- [ ] **步骤 5：运行测试**

```powershell
cd backend
go test ./internal/api ./internal/security ./internal/storage -count=1
```

预期：PASS。

### 任务 4：增加前端系统配置 UI

**文件：**
- 修改：`frontend/lib/api-types.ts`
- 修改：`frontend/lib/system-settings.ts`
- 修改：`frontend/app/system-settings-page.tsx`

- [ ] **步骤 1：扩展类型与默认值**

```ts
export interface SystemSettings {
  // existing fields...
  iframe_embedding_enabled: boolean
  iframe_embedding_mode: "all" | "allowlist"
  iframe_embedding_allowlist: string[]
}
```

默认：

```ts
iframe_embedding_enabled: false,
iframe_embedding_mode: "allowlist",
iframe_embedding_allowlist: [],
```

- [ ] **步骤 2：新增 UI**

在系统配置页增加“安全与嵌入” Card：

- `Switch`：“允许 iframe 嵌入”
- 开启后 `Select`：“允许所有 / 指定白名单”
- 白名单模式显示 `Textarea`，每行一个规则
- 输入时按换行转成 `string[]`，trim 空行
- 显示简短规则说明：
  - `*` 所有
  - `*.example.com` 单层
  - `**.example.com` 多层
  - `a*abc.com` label 内字符串
  - `**.devai.**` 任意多层包含完整 `devai` label

- [ ] **步骤 3：前端静态检查**

```powershell
cd frontend
pnpm lint
pnpm build
```

预期：两者 exit 0。

### 任务 5：集成验证

- [ ] **步骤 1：后端完整测试**

```powershell
cd backend
go test ./...
```

预期：exit 0。

- [ ] **步骤 2：格式与 diff 检查**

```powershell
gofmt -w backend/internal/security/hostglob.go backend/internal/security/hostglob_test.go backend/internal/api/frame_policy.go backend/internal/api/frame_policy_test.go backend/internal/storage/settings_test.go backend/internal/storage/model.go backend/internal/storage/settings.go backend/internal/api/system_settings.go backend/internal/api/api.go
git diff --check
```

预期：无输出错误。

- [ ] **步骤 3：逐项核对行为**

确认：

- 默认关闭
- all 模式
- allowlist 多条
- `*`
- `*.example.com`
- `**.example.com`
- `a*abc.com`
- `*.devai.*`
- `**.devai.**`
- 白名单来源缺失 fail closed
- 不影响直接顶层访问
- 不修改 API/SSE 响应策略
- 保存后无需重启即可生效

- [ ] **步骤 4：只检查本功能 diff**

由于工作区已有大量未提交改动，完成后用：

```powershell
git diff -- backend/internal/security backend/internal/api/frame_policy.go backend/internal/api/frame_policy_test.go backend/internal/api/system_settings.go backend/internal/api/api.go backend/internal/storage/settings.go backend/internal/storage/settings_test.go backend/internal/storage/model.go frontend/lib/api-types.ts frontend/lib/system-settings.ts frontend/app/system-settings-page.tsx
```

人工确认没有覆盖既有功能。
