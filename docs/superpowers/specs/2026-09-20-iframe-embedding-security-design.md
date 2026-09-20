# GatewayOps iframe 嵌入系统级配置设计

日期：2026-09-20

## 1. 背景

GatewayOps 现有“系统配置”已经通过 `app_settings` 表持久化页面级开关和选项，但当前仓库中没有显式的 `X-Frame-Options`、`Content-Security-Policy: frame-ancestors` 或反向代理 `add_header` 配置。

分组监控页面嵌入 Sub-API 时曾出现 iframe 无法展示的问题。为了让 iframe 行为由 GatewayOps 自身可控，而不是依赖外部 Nginx、Cloudflare 或其他反向代理，需要增加一组系统级安全配置。

## 2. 目标

在“系统配置”中新增 iframe 嵌入控制，满足：

1. 默认关闭，保持安全默认值：任何第三方页面都不能 iframe 嵌入 GatewayOps。
2. 开启后可以选择：
   - 允许所有来源嵌入；
   - 仅允许白名单来源嵌入。
3. 白名单支持多条规则。
4. 白名单支持精确域名、单层通配、多层通配以及域名段内字符串通配。
5. 配置修改后无需重启服务，后续请求立即读取最新配置并生效。
6. iframe 限制由 GatewayOps 应用层负责，避免部署环境差异导致行为不一致。

## 3. 配置模型

新增三个系统设置字段：

- `iframe_embedding_enabled: boolean`
  - 默认 `false`
  - `false` 时拒绝所有 iframe 嵌入
- `iframe_embedding_mode: "all" | "allowlist"`
  - 仅在 enabled=true 时生效
  - 默认 `allowlist`
- `iframe_embedding_allowlist: string[]`
  - 多条域名 Glob 规则
  - 仅在 mode=allowlist 时生效

后端持久化到现有 `app_settings`，建议 key：

- `security.iframe.enabled`
- `security.iframe.mode`
- `security.iframe.allowlist`

其中 allowlist 使用 JSON 数组序列化保存。

## 4. 白名单 Glob 语义

匹配对象是 iframe 父页面 URL 的 hostname，不匹配路径。

规则统一转为小写后匹配；域名末尾多余的 `.` 先去掉。

### 4.1 基本规则

| 规则 | 含义 |
| --- | --- |
| `*` | 允许所有域名 |
| `example.com` | 仅匹配 `example.com` |
| `*.example.com` | 仅匹配一层子域名，例如 `a.example.com` |
| `**.example.com` | 匹配任意层级子域名，但不包含根域名 |
| `a*abc.com` | `*` 在当前域名段内匹配任意字符，但不跨 `.` |
| `*.devai.*` | `devai` 前后各恰好一层 |
| `**.devai.**` | `devai` 前后允许任意层级 |

### 4.2 星号定义

- `*`
  - 匹配当前 DNS label 内任意数量字符，包括 0 个字符
  - 不跨越 `.`
  - 当整个 label 就是 `*` 时，表示恰好一个域名层级
- `**`
  - 仅允许作为完整 DNS label 使用
  - 匹配一个或多个域名层级
  - 不表示 0 层，因此 `**.example.com` 不匹配 `example.com`

### 4.3 示例

`a*abc.com`：

- ✅ `aabc.com`
- ✅ `axabc.com`
- ✅ `a123abc.com`
- ❌ `a.fooabc.com`

`*.example.com`：

- ✅ `a.example.com`
- ❌ `a.b.example.com`
- ❌ `example.com`

`**.example.com`：

- ✅ `a.example.com`
- ✅ `a.b.example.com`
- ❌ `example.com`

`*.devai.*`：

- ✅ `a.devai.com`
- ✅ `x.devai.net`
- ❌ `a.b.devai.co.uk`

`**.devai.**`：

- ✅ `a.devai.com`
- ✅ `a.b.devai.c.d`
- ✅ `foo.bar.devai.co.uk`
- ❌ `mydevai.com`
- ❌ `devai-test.com`

如果需要根域名本身，必须显式增加对应规则。例如：

```text
example.com
**.example.com
```

## 5. 为什么不能直接把 Glob 写入 CSP

浏览器原生 `Content-Security-Policy: frame-ancestors` 能表达精确 origin 和有限的主机通配，但不能表达：

- `a*abc.com`
- `*.devai.*`
- `**.devai.**`

因此 GatewayOps 不能把用户配置原样输出为 CSP。

设计采用“应用层匹配 + 浏览器层精确约束”：

1. 浏览器发起 iframe 文档请求；
2. GatewayOps 读取该请求的父页面来源；
3. GatewayOps 用自定义 Glob 匹配父页面 hostname；
4. 匹配成功时输出父页面的精确 origin 到 `frame-ancestors`；
5. 匹配失败时输出 `frame-ancestors 'none'`。

这样高级 Glob 由 GatewayOps 负责表达，最终拦截仍由浏览器 CSP 执行。

## 6. 父页面来源判定

优先使用请求 `Referer` 解析父页面 origin。

同时结合 Fetch Metadata：

- `Sec-Fetch-Dest: iframe` / `frame`
- `Sec-Fetch-Site`

用于辅助判断当前请求是否为嵌入导航。

白名单模式下：

- 能解析出合法父页面 origin 且 hostname 命中规则：允许
- Referer 缺失、非法或无法确定父来源：拒绝

这是 fail-closed 行为。

因此，如果父站点主动使用 `Referrer-Policy: no-referrer` 或 iframe 的 `referrerpolicy="no-referrer"`，白名单模式会拒绝该嵌入。允许所有模式不依赖 Referer。

## 7. 响应头策略

### 7.1 关闭

```http
Content-Security-Policy: frame-ancestors 'none'
X-Frame-Options: DENY
```

目标：所有站点都无法嵌入。

### 7.2 开启 + 允许所有

不设置 iframe 限制型 `frame-ancestors`，同时不设置 `X-Frame-Options`。

目标：允许所有父页面。

### 7.3 开启 + 白名单

父来源命中后，例如父页面：

```text
https://sub.example.com:8443/path
```

响应：

```http
Content-Security-Policy: frame-ancestors https://sub.example.com:8443
```

不再发送 `X-Frame-Options`，避免其较老语义与 CSP 白名单冲突。

不命中时：

```http
Content-Security-Policy: frame-ancestors 'none'
```

## 8. 作用范围

iframe 安全中间件挂在 GatewayOps 的页面响应层，至少覆盖：

- `/`
- SPA 路由，如 `/channels`、分组监控页面等
- 前端静态入口文档

API JSON、SSE、健康检查无需依赖 iframe 策略。

由于 `frame-ancestors` 不影响正常顶层打开页面，关闭 iframe 不会影响用户直接访问 GatewayOps。

## 9. 前端系统配置 UI

在“系统配置”页面新增独立卡片：**安全与嵌入**。

配置结构：

1. “允许 iframe 嵌入”开关
2. 开启后显示“允许来源”选项：
   - 允许所有
   - 指定白名单
3. 选择白名单后显示多行规则编辑器
4. 每行一条规则，也允许粘贴多行
5. 保存前在前端做基础语法校验
6. 后端再次做完整校验，后端为最终事实源

UI 中直接展示规则说明和示例：

```text
*
example.com
*.example.com
**.example.com
a*abc.com
*.devai.*
**.devai.**
```

## 10. 规则校验

拒绝以下配置：

- 空白规则
- 带 URL path 的规则，如 `example.com/foo`
- 带 scheme 的规则，如 `https://example.com`
- 带端口的规则，如 `example.com:8443`
- 连续空 label，例如 `a..example.com`
- `**` 作为 label 内一部分，例如 `ab**cd.example.com`
- 非法域名字符

允许：

- ASCII 域名
- `localhost`
- IPv4 / IPv6 精确值可以后续扩展；首版不纳入 Glob 规则，避免语义混乱

白名单匹配只匹配 hostname；scheme 和 port 不参与白名单判断。命中后 CSP 输出请求中实际父页面的完整 origin，因此实际 scheme/port 会被精确约束。

## 11. 后端结构建议

拆成三个职责：

### 11.1 Settings 持久化

扩展现有 `SystemSettings` / `UISettings` 或将其重命名为更通用的系统设置模型。

由于该页面现在已经不再只是 UI 信息密度设置，建议将后端类型从 `UISettings` 演进为 `SystemSettings`，同时保留现有 API 路径 `/api/system-settings`。

### 11.2 Host Glob matcher

独立纯函数模块，职责：

- 规则解析
- 规则校验
- hostname 匹配

不依赖 Gin、数据库或 HTTP，便于完整单元测试。

### 11.3 iframe middleware

职责：

- 读取当前系统设置
- 判断模式
- 解析父页面来源
- 调用 matcher
- 设置对应响应头

不负责 Glob 具体算法。

## 12. 缓存与即时生效

首版直接从现有 Settings 层读取配置即可。

如果担心每个页面请求读取数据库，可在 Settings 层增加轻量内存缓存，并在 `SaveSystemSettings` 后原子更新缓存。

无论采用哪种方式，都必须保证保存后无需重启。

## 13. 测试

### 13.1 Matcher 单元测试

至少覆盖：

- 精确域名
- `*`
- 单层 `*`
- 多层 `**`
- label 内 `*`
- `*.devai.*`
- `**.devai.**`
- 大小写
- 非法规则
- 根域名不被 `*.example.com` / `**.example.com` 意外匹配

### 13.2 Middleware 测试

覆盖：

- 默认关闭返回 DENY + frame-ancestors none
- allow-all 不输出限制头
- allowlist 命中输出精确父 origin
- allowlist 未命中输出 none
- Referer 缺失时 fail closed
- 带端口父 origin
- 顶层访问不受 iframe 策略影响

### 13.3 API / Settings 测试

覆盖：

- 默认值
- 保存/读取
- 多条规则持久化
- 非法规则返回 400
- 保存后即时生效

## 14. 安全边界

该功能只负责 GatewayOps 自己输出的 iframe 限制。

如果部署环境中的 Nginx、Caddy、Cloudflare Transform Rules、WAF 或其他代理额外添加：

- `X-Frame-Options: DENY`
- `X-Frame-Options: SAMEORIGIN`
- 更严格的 `Content-Security-Policy: frame-ancestors ...`

浏览器仍可能被外部响应头阻止。

因此应用配置开启后，如果实际部署仍无法嵌入，需要检查最终公网响应头，而不是只检查 GatewayOps 配置。

## 15. 非目标

首版不实现：

- 按具体 GatewayOps 页面分别配置 iframe 白名单
- 按用户/角色配置不同白名单
- 正则表达式规则
- URL path 级白名单
- scheme / port 作为白名单输入条件
- 自动修改 Nginx / Cloudflare 配置

这些能力后续有明确需求再扩展。
