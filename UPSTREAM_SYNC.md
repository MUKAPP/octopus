# 上游同步台账

本仓库是基于 `bestruirui/octopus` 的长期 fork。`dev` 是本 fork 的主开发分支；上游当前主分支为 `master`。

## 同步边界

本 fork 保留独立的调度器、balancer、渠道/分组模型及相关数据库结构。上游涉及以下内容时，不直接合并其结构性改动：

- 调度方式、故障转移、倍率/优先级调度；
- 渠道模型拆表或其他相关数据库 schema 重构；
- 依赖上述改动的 relay、超时、统计、迁移及前端提交。

这些提交如果包含独立的通用修复，应在本 fork 的现有架构上单独适配，并记录本地提交与上游 SHA 的对应关系。

## 远程与分支约定

```text
origin          https://github.com/MUKAPP/octopus       本 fork
origin/dev      本 fork 主开发线
upstream        https://github.com/bestruirui/octopus   上游只读参考
upstream/master 上游最新状态
```

更新上游引用：

```bash
git fetch upstream --prune
```

每批同步使用临时分支，不直接在 `dev` 上处理：

```bash
git switch -c sync/upstream-YYYY-MM-DD dev
# 评估、适配、验证后：
git switch dev
git merge --no-ff sync/upstream-YYYY-MM-DD
```

## 审查状态定义

- **已引入**：上游改动已经进入本 fork，记录本地提交 SHA。
- **适配引入**：功能已保留，但实现基于本 fork 架构，记录上游和本地 SHA。
- **部分引入**：只采用提交中的独立部分，明确列出未采用部分。
- **不引入**：已确认与 fork 架构或产品方向冲突。
- **暂缓**：尚未完成判断，下一批同步必须重新处理。

“已审查”不等于“已引入”。不要通过只接入上游历史、但不改变文件树的合并提交来推进审查进度。

## 审查检查点

| 项目 | 状态 |
|---|---|
| 上次历史审查的上游提交 | `dddc6bc1f26f13ac408d86806dbdcf984e1da2ec` |
| 上次历史检查点对应本地提交 | `5fc512ed6220d002c2a529825b158c56594226c0` |
| 说明 | 历史检查点只标记已审查，不代表此前上游提交全部进入当前文件树 |
| 本批固定参考上游快照 | `357787d56b984f0dc985ca9b2792b100a814cdc9`（本地已有引用，本批未 fetch） |
| 本次已审查至 | `27aa40dc0f3b2902bce3e96ccdba019d17041606` |
| 本次审查所在本地基线 | `dev` @ `3d606b01a7c06dad27c3246e2fe69afd92073212` |
| 本次新增上游代码合入 | 无；本次只审查，没有 cherry-pick 或 merge |
| 待审查提交数量 | 0 |

## 已审查提交

历史范围：`dddc6bc..27aa40d`。另记录 2026-09-30 参考 `upstream/master` @ `357787d` 的非连续选择性适配：首批三项以 `dev` @ `6d56e7a` 为基线，剩余四项以 `dev` @ `16d7bab` 为基线；经用户后续明确授权，均已分别提交至 `dev`。以下结论针对当前文件树；“等价补丁”表示补丁内容与本地提交相同，“功能覆盖/功能适配/独立实现”表示功能已存在但原始上游 SHA 和补丁均未进入 Git 历史。此次手工适配不引入上游原始提交历史，也不推进连续审查检查点。

| 上游提交 | 处理结果 | 本地提交 | 备注 |
|---|---|---|---|
| `6e736b4` | 独立实现（原提交未引入） | `ae3d0c5`, `3d606b0` | 当前 fork 早已有 `Channel.Keys` 与 `channel_keys`，并保留自己的 key 选择逻辑；未引入上游 `ChannelGrant`、迁移 011/012 及协议/授权模型 |
| `1be0acd` | 等价引入 | `de36a05` | `git cherry -v dev upstream/master` 标记为 `-`；两者 patch-id 相同，均移除 auth middleware 中相同的 6 行前缀校验 |
| `1ab744f` | 功能覆盖（非原补丁） | `12cd0be` | 当前 `setupLogSSE` 在日志快照前写入 `: connected` 并 Flush，覆盖空日志流即时刷新；实现早于该上游提交且不是同一 patch |
| `f07b8dc` | 未引入 | — | 仅把 `main.go` 版本注释改为 `v0.13.0`；当前 `main.go` 没有该版本注释 |
| `13a144a` | 不引入 | — | 按项目决定不引入 Issue 模板 AI 撰写确认项和 `template-check.yaml`；属于仓库治理，不影响运行时产品能力 |
| `3364bb1` | 前端部分未采用；axonhub 由 CI 覆盖 | — | 不复制上游前端依赖/锁文件/编译链；axonhub 最新版继续由既有 CI `go get github.com/looplj/axonhub/llm@unstable` 解析，本批保留本地伪版本；Go 的选择性升级另见本批 `1bd2ed8` / `9f91bad` 记录 |
| `e3af162` | 功能适配（非原补丁） | `1d1e722` | 当前静态中间件和 Vite 构建配置均支持 gzip；本地版本另外实现 q 值/wildcard 协商与 406，patch-id 不同 |
| `dd21e66` | 未引入 | — | 上游只修复 `internal/db/migrate/005.go`；当前 fork 迁移目录只有 001–004，目标迁移/schema 不在当前架构中 |
| `7bbf77d` | 部分引入（手工适配） | `6dda230` | 仅创建表单防误关/取消按钮；其他弹窗默认外点关闭不变；未引入 ChannelGrant、日志及其余结构性优化 |
| `27aa40d` | 未引入 | — | 仅把 `main.go` 版本注释从 `v0.13.1` 改为 `v0.13.2`；当前没有该注释 |
| `1c48ee5` | 适配引入 | `eaa5a12` | 请求头占位符读取原始客户端入站头；受支持压缩解码前快照；保持渠道认证目标头优先；模型探测不解析占位符 |
| `d5a893f` | 适配引入 | `0a4482b` | 全局 ECMAScript 正则仅过滤手动获取模型，与渠道过滤串联（AND）；自动同步及分组删除路径不变 |
| `bf2027a2c9e54182dadf636166b8f10c22fd90b8` | 部分引入（仅开始时刻，手工适配） | `3add2ca` | 真实尝试新增 `started_at_ms`（Unix 毫秒），在 fork 的模型 JSON、双 SSE 与前端详情透传；跳过条目省略字段/显示占位；不采用上游 spinner 配色或 RequestState 状态机，无 DB 列/迁移 |
| `aa07f77ab8d12f691dd43baaa77cbe5766f4f012`、`357787d56b984f0dc985ca9b2792b100a814cdc9`（预设快照） | 适配引入（单主协议预填） | `c5e91a4` | 创建态 17 项服务商模板只预填现有 `type` / `base_urls`；名称为空时本地化填入，已有名称/密钥/模型及其他草稿保留；空地址模板保留整个地址数组和 delay。保留 DeepSeek `#` 跳过 `/v1`，复用火山 v3/智谱 v4 既有 URL 规则；不引入 dialect、ChannelGrant、多协议路径或 schema |
| `08d80c365dcd713203f061c46677399cae3ba5f5` | 适配引入（几何及缓存刷新） | `5655825` | React Logo、HTML loader、亮/暗 SVG 统一三路径及顺序；保留 fork 0.8s 绘制/0.15s 错峰/0.6s 淡出和既有 reduced-motion；计算门控为 1100ms。SW v2→v3 刷新同名 SVG，font 缓存/协议不变；未重绘 PNG/favicon |
| `1bd2ed8de9e0dd44898f0486c4ad7259e10e4028`、`9f91bad5ff1e1da90e326254c818afb8b63e2bae` | 适配引入（四依赖及必要闭包） | `4e10525` | 模块管理器升级 cors v1.7.9、crypto v0.57.0、net v0.59.0、postgres v1.6.3；接受 MVS 必需的 Gin v1.12.0、gorm v1.31.2、pgx v5.10.0、sys v0.48.0/text v0.42.0 等闭包。保留 Go 1.26.0、axonhub/zap 和两项 replace，无业务 API/schema 迁移 |
| `44f63cb` | 未引入 | — | 整请求取消不在本批范围；既有单次尝试停止路径保留 |
| `00e0739`、`a0a5c66`、`d18a419`（实时计数部分） | 未引入 | — | 流式实时输出计数不在本批范围，不将上游事件计数解释为 token/字符数 |

## 同步批次记录

| 日期 | 上游范围 | 结果 | 本地合并/提交 | 备注 |
|---|---|---|---|---|
| 本轮审查 | `dddc6bc..27aa40d` | 10 个提交均已判定；未新增代码合入 | `dev` @ `3d606b0` | 后续从 `27aa40d` 之后的上游提交开始评估 |
| 2026-09-30 | 选择性适配 `1c48ee5`、`d5a893f`、`7bbf77d` 独立表单部分；参考 `357787d` | 三项适配已分别提交至 `dev`；未 cherry-pick/merge 上游历史 | `eaa5a12`、`0a4482b`、`6dda230`；基线 `6d56e7a` | 在 `sync/upstream-2026-09-30` 实施验证后，同基线带回 `dev`；用户明确要求在保留下述全量检查失败说明的情况下提交交付；连续审查点仍为 `27aa40d` |
| 2026-09-30（剩余四项） | 选择性适配 `bf2027a`、`aa07f77` / `357787d` 预设快照、`08d80c3`、`1bd2ed8` / `9f91bad`；固定参考 `357787d` | 四项适配已分别提交至 `dev`；保留 fork 调度/渠道模型；运行验证及只读代码审查完成，仍有下述既有检查限制 | `3add2ca`、`c5e91a4`、`5655825`、`4e10525`；基线 `dev` @ `16d7bab` | 在 `sync/upstream-2026-09-30-remaining` 实施验证后，同基线带回 `dev`；经用户后续明确授权分别提交；未接入上游历史，连续审查点仍 `27aa40d` |

### 2026-09-30 验证结果

- `pnpm install --frozen-lockfile`、`pnpm build`（TypeScript + Vite）成功；未升级依赖。构建产物仅用于运行验证，不纳入提交。
- `go test ./...`：本批相关 relay/model/handler 及 helper/op/task 等包通过；`internal/db.TestCompactSQLite` 失败，Windows 临时目录同步返回 `Access is denied`（`compact_test.go:159`）。本批未修改该路径，未为此扩展修复范围。
- `pnpm lint`：22 错误、1 警告，涉及既有 animate-ui、hooks/provider、Fast Refresh 非组件导出等；无新增代码行诊断。未抑制规则或重构无关组件。
- 独立临时 SQLite、127.0.0.1 后端/fixture 的真实 API smoke 通过：原始入站头引用、非递归替换、认证优先及 gzip 解码前编码/长度快照；过滤顺序、AND、无匹配空数组、清空恢复、无效正则 HTTP 400 且原设置保留；全局过滤不影响自动同步的三模型和既有分组成员。旧库默认补齐仅核对既有代码路径，不称为本批升级实测。
- 本轮构建后嵌入页面的独立 Chromium smoke 通过：设置失焦保存/刷新、错误提示与草稿保留、有效重试及清空；三类创建外点保留、取消无创建、X/Escape 焦点恢复、Tab 圈及渠道 Select/创建成功；渠道详情、分组编辑、API Key 面板、日志详情仍默认外点关闭。1280×900 与 390×844、三语言检查无本批横向溢出或底部取消遮挡。
- 经用户明确授权，在保留上述验证限制的情况下提交到 `dev`；未 cherry-pick 或 merge 上游历史，不将其他上游改动标为已引入，不推进连续审查检查点。

### 2026-09-30 剩余四项适配验证（已提交至 dev）

- `go get` 四个精确目标版本、`go mod tidy` 完成；未运行 axonhub `@unstable`，前端依赖/锁文件不变。`go test ./... -skip '^TestCompactSQLite$'` 通过（11 个测试包、16 个无测试包）；按计划跳过已报告的 Windows 目录 sync 失败用例，不称为未跳过的全量测试全绿。
- `pnpm build`（TypeScript + Vite）及 `go build -trimpath -tags=jsoniter` 成功。`pnpm lint` 仍为基线的 22 错误、1 警告：14 个诊断文件与基线逐字一致，Logo 的非组件导出诊断对应已有 `LOGO_DRAW_END_MS`，无本批新增代码行诊断；不抑制规则或扩展修复范围。
- 隔离临时 SQLite/本地 HTTP fixture：bcrypt 错误密码 401、正确密码 cookie 登录、渠道与分组创建/查询通过。跳过渠道没有上游请求和 `started_at_ms`，两次真实尝试开始时刻递增，overview 的 history/attempts、详情 started/finished 及终态原值一致，fallback 客户端 200/`ok`。
- 17 项预设逐项真实创建渠道和单成员分组，以 fixture 替换 authority、保留路径及 DeepSeek `#`，出站路径全部符合各预设的主协议；Anthropic `/v1/messages` 转为 chat、火山 v3、智谱 v4、DeepSeek 无 `/v1`、Azure `/openai/v1` 均得到客户端 200/`ok`。仅证明本 fork 拼接/转换，不声称已验证厂商真实服务或其他协议支持。
- 构建后独立 Chromium：17 模板逐项 Enter/Space 激活，零 create/fetch-model 请求；草稿保留、非空地址整组替换、New API/Azure 空地址保留、手动协议不重写地址及重复应用通过。真实提交 body 正确且列表出现实体；重复名错误 toast 保留草稿；编辑无模板。1280×900 / 390×844、简/繁/英、亮/暗共 12 轮检查无本批横向溢出，滚动底部按钮可达，默认折叠、外点保留、取消、Select portal、首尾 Tab 圈和 Escape 保留。
- 日志运行态、终态及刷新重开显示同一服务端时刻；详情尝试行、选中响应头、重试 tooltip 的时间/占位一致，窄屏整块换行。以真实捕获的 model JSON 和临时模拟 overview 路由缺失核验旧列表分支，第四处显示同值；验证拦截退出后已恢复真实网络。实际 formatter 会话检查覆盖缺失/零/非法日期占位及毫秒补三位，无新增或改写永久测试。
- Logo 加载态/AppShell/登录页及两 SVG 三路径一致；实际 React 绘制门控约 1123ms（计算值 1100ms），正常结束。reduced-motion CSS 首屏 loader 静态完整显示，React 既有 reduced-motion 行为保留。真实 SW 从 seed 的 `octopus-shell-v2` 激活至 v3、旧 v2 缓存删除；离线读取两 SVG 为新三路径，固定资源重缓存、`octopus-font` 内容保留。
- CORS 空值/拒绝 origin 返回 403；`*`、完整 origin、host 匹配返回 204，回显 origin 且 Allow-Credentials=true。管理 overview/detail SSE 读首帧后主动断连不取消随后成功请求；relay stream 读首内容后关闭，终态 canceled，下一请求仍 200，无新增 panic/recovery 异常。
- 真实隔离 `postgres:17` 容器：应用初始化/AutoMigrate、登录、创建渠道/分组、连续两轮 list 成功；重启应用复用临时库，实体仍在、无 prepared-statement/cached-plan 错误。未使用生产 DSN，不声称已验证 PgBouncer。
- 本批服务、fixture、浏览器和唯一 PG 容器已停止，应用/fixture 端口已证明释放；不触碰仓库 `data/`、用户凭据或其他容器。按授权启动的 Docker Desktop 保留运行。截图和会话证据保留于本地忽略记忆，不纳入提交。
- 本批不是 `27aa40d` 之后 25 提交的连续完整审查，不推进连续审查点；整请求取消与流式实时计数仍未引入。四个 reviewer 只读审查及主代理关键定位核验未发现新增可行动缺陷；经用户后续明确授权，四项已按上表真实本地 SHA 分别提交至 `dev`，保留上述既有检查限制。



## 推荐评估命令

评估自上次检查点以来的提交：

```bash
git log --reverse --stat --oneline <last-reviewed-upstream-sha>..upstream/master
git show <upstream-sha>
```

查看当前 fork 与上游的差异：

```bash
git log --left-right --cherry-pick --oneline dev...upstream/master
git diff --stat dev...upstream/master
```

记录某个上游提交的适配来源：

```bash
git cherry-pick -x <upstream-sha>
```

如果是手工适配，不使用 cherry-pick，也要在本地提交正文中写明：

```text
Upstream: <upstream-sha>

Adapted to the fork's retained scheduling and channel model.
```

每次完成一批同步后：

1. 更新上表中的每个上游提交状态；
2. 记录对应的本地提交；
3. 明确仍未采用的部分和暂缓原因；
4. 运行受影响的验证命令；
5. 只有所有提交都有明确结论后，才把检查点更新到新的上游 SHA。
