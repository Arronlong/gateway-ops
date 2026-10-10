# 渠道编辑中的中转站账号关联实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将中转站账号关联能力接入渠道编辑弹窗，让用户可以在编辑已有渠道时按渠道账号配置中转站账号，并让关联快照刷新后驱动列表筛选。

**架构：** 保留现有 `/channels/associations` 查询和 `PUT /channels/:id/accounts/:account_id/relay-links/:station_id` 更新接口。把当前未挂载的账号关联 UI 抽取为独立组件，由 `ChannelFormDialog` 在编辑模式下渲染；父级复用已有关联快照和刷新函数，避免重复请求。

**技术栈：** React 19、TypeScript、Vite、Radix Select、现有 `apiFetch` 和 `useChannelRelayAssociations`。

---

### 任务 1：补充关联编辑数据与 UI 行为测试

**文件：**
- 创建：`frontend/lib/channel-relay-association.test.ts`
- 创建：`frontend/lib/channel-relay-association.ts`

- [x] **步骤 1：编写失败测试**

测试纯函数 `associationValue`：

```ts
test("manual linked association uses the selected relay account", () => {
  expect(associationValue({ mode: "manual", linked: true, relay_account_external_id: 42 })).toBe("manual:42")
})

test("missing or automatic association falls back to auto", () => {
  expect(associationValue(undefined)).toBe("auto")
  expect(associationValue({ mode: "auto", linked: false, relay_account_external_id: 0 })).toBe("auto")
})
```

同时测试 `linkedStationsByChannel` 只收集 `linked === true` 的中转站，失效或未关联记录不能命中具体中转站筛选。

- [x] **步骤 2：运行测试确认失败**

运行：`node --experimental-strip-types --test frontend/lib/channel-relay-association.test.ts`

预期：因测试目标函数尚未创建而失败，失败原因应是模块或导出不存在，而不是配置解析错误。

- [x] **步骤 3：实现最小纯函数**

在 `frontend/lib/channel-relay-association.ts` 中提供关联选择值转换和渠道级中转站聚合函数，只处理数据转换，不引入 React 或 API 调用。

- [x] **步骤 4：运行测试确认通过**

运行：`node --experimental-strip-types --test frontend/lib/channel-relay-association.test.ts`

预期：新增测试全部通过。

### 任务 2：抽取中转站账号关联编辑区域

**文件：**
- 创建：`frontend/components/monitor/channel-relay-association-editor.tsx`
- 修改：`frontend/components/monitor/channel-cards.tsx`

- [x] **步骤 1：迁移现有关联选择逻辑**

从 `channel-cards.tsx` 迁移 `ChannelAssociationDialog` 内的账号遍历、状态显示和 PUT 请求逻辑，形成 `ChannelRelayAssociationEditor`。组件接收：

```ts
type ChannelRelayAssociationEditorProps = {
  channel: Channel
  snapshot: ChannelRelayAssociationSnapshot | null
  disabled?: boolean
  onChanged: () => Promise<void>
}
```

每个账号和中转站保留三种选择：`auto`、`manual:none`、`manual:<external_id>`。

- [x] **步骤 2：运行类型检查确认迁移期间问题可定位**

运行：`pnpm --dir frontend exec tsc --noEmit`

预期：若组件迁移尚未完成，输出只允许是关联组件的导入、属性或未使用符号错误；修复后继续下一步。

- [x] **步骤 3：删除未挂载的旧组件和状态**

从 `channel-cards.tsx` 删除未使用的 `AssociationStatusButton`、`ChannelAssociationDialog`、`associating` 状态以及仅为它们服务的 Dialog/Select 类型导入。保留列表筛选依赖的 `useChannelRelayAssociations` 和关联快照聚合逻辑。

### 任务 3：把关联区域接入编辑渠道弹窗

**文件：**
- 修改：`frontend/components/monitor/channel-form-dialog.tsx`
- 修改：`frontend/components/monitor/channel-cards.tsx`

- [x] **步骤 1：扩展 `ChannelFormDialog` 属性**

新增可选属性：

```ts
associationSnapshot?: ChannelRelayAssociationSnapshot | null
onAssociationChanged?: () => Promise<void>
```

只有 `channel`、`associationSnapshot` 和 `onAssociationChanged` 同时有效时才显示关联区域，保证新增渠道不显示该功能。

- [x] **步骤 2：在编辑弹窗中渲染关联区域**

将 `ChannelRelayAssociationEditor` 放在渠道账号编辑区域之后、错误提示和底部按钮之前；将弹窗宽度从 `sm:max-w-md` 调整为适合中转站账号列表的 `sm:max-w-3xl`，保持移动端垂直滚动。

- [x] **步骤 3：父级传递已有快照和刷新函数**

在 `ChannelCards` 的 `ChannelFormDialog` 调用处传入：

```tsx
associationSnapshot={associations.data ?? null}
onAssociationChanged={associations.refetch}
```

编辑渠道保存完成后的全局刷新仍走原有 `refresh()`，关联选择则即时刷新关联快照，不将关联数据混入渠道主体保存 payload。

### 任务 4：验证完整用户链路

**文件：**
- 验证：`frontend/components/monitor/channel-form-dialog.tsx`
- 验证：`frontend/components/monitor/channel-relay-association-editor.tsx`
- 验证：`frontend/components/monitor/channel-cards.tsx`
- 验证：`backend/internal/storage/channel_relay_links.go`

- [x] **步骤 1：运行前端测试和类型检查**

运行：

```bash
node --experimental-strip-types --test frontend/lib/channel-relay-association.test.ts
pnpm --dir frontend exec tsc --noEmit
```

预期：测试通过，TypeScript 无错误。

- [x] **步骤 2：运行前端构建**

运行：`pnpm --dir frontend build`

预期：Vite 构建成功退出，未出现组件导入、JSX 或类型错误。

- [x] **步骤 3：核对后端失效关联逻辑**

确认 `ChannelRelayAssociationSnapshot()` 调用 reconcile；当 `relay_account_external_id` 不再存在时删除失效链接，使列表聚合不再命中具体中转站，筛选“未关联”可以命中该渠道。

- [x] **步骤 4：重启服务**

运行：`C:\Users\cclsu\Desktop\gatewayops\start.bat`

预期：脚本完成服务重启；若脚本保持前台运行，则记录进程状态并继续轮询验证。

- [x] **步骤 5：检查交付文件编码和变更范围**

检查计划、设计文档和代码文件中不存在连续问号或 U+FFFD；确认没有修改用户已有的无关变更。
