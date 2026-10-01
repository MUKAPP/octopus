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
| 本批固定参考上游快照 | `0538c3e715e529e3a1f3a2b1229addb8ca4357ea`（2026-10-01 fetch；此前选择性适配参考 `357787d`） |
| 本次已审查至 | `0538c3e715e529e3a1f3a2b1229addb8ca4357ea` |
| 本次审查所在本地基线 | `dev` @ `8b1dc923f171fe8aaea7e607f872dac04910f11c` |
| 本次新增上游代码合入 | 无；本次只审查，没有 cherry-pick 或 merge |
| 待审查提交数量 | 0 |

## 已审查提交

历史范围：`dddc6bc..27aa40d`；2026-10-01 已完整续审 `27aa40d..0538c3e` 的 29 个提交，逐提交结论见下方独立章节。另记录 2026-09-30 参考 `upstream/master` @ `357787d` 的非连续选择性适配：首批三项以 `dev` @ `6d56e7a` 为基线，剩余四项以 `dev` @ `16d7bab` 为基线；经用户后续明确授权，均已分别提交至 `dev`。以下结论针对当前文件树；“等价补丁”表示补丁内容与本地提交相同，“功能覆盖/功能适配/独立实现”表示功能已存在但原始上游 SHA 和补丁均未进入 Git 历史。此前手工适配不引入上游原始提交历史；此次连续审查点推进只代表完成审查，不代表合入。

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
| `8e93fa6fbc90383f765f32bf9a611604eb6a191b` | 适配引入（渠道 Key 快照与标识） | `5156f9c` | 当次真实尝试保存 ID 与 trim 后备注，双 SSE/终态/旧协议五处独立文本展示；不泄露密钥、不污染渠道统计、不新增 schema，长备注重试提示限定滚动增强 |
| `bdc99486b0b6f400633dcaeedf415927c9805ff8` | 适配引入（子目录与 PWA 隔离） | `3afa382` | API、导入导出、三 SSE、SW 统一应用目录；shell/static 与强刷新按目录隔离，字体共享，旧 root 缓存精确迁移；不新增认证隔离 |
| `0538c3e715e529e3a1f3a2b1229addb8ca4357ea` | 适配引入（创建/编辑成员首尾移动） | `8b1110a` | 编辑器按钮仅改草稿，保存沿用 priority；列表卡片保留原 DnD，空/单成员及拖拽删除禁用；不改调度/schema |
| `44f63cb` | 未引入 | — | 整请求取消不在本批范围；既有单次尝试停止路径保留 |
| `00e0739`、`a0a5c66`、`d18a419`（实时计数部分） | 未引入 | — | 流式实时输出计数不在本批范围，不将上游事件计数解释为 token/字符数 |

## 同步批次记录

| 日期 | 上游范围 | 结果 | 本地合并/提交 | 备注 |
|---|---|---|---|---|
| 本轮审查 | `dddc6bc..27aa40d` | 10 个提交均已判定；未新增代码合入 | `dev` @ `3d606b0` | 后续从 `27aa40d` 之后的上游提交开始评估 |
| 2026-09-30 | 选择性适配 `1c48ee5`、`d5a893f`、`7bbf77d` 独立表单部分；参考 `357787d` | 三项适配已分别提交至 `dev`；未 cherry-pick/merge 上游历史 | `eaa5a12`、`0a4482b`、`6dda230`；基线 `6d56e7a` | 在 `sync/upstream-2026-09-30` 实施验证后，同基线带回 `dev`；用户明确要求在保留下述全量检查失败说明的情况下提交交付；连续审查点仍为 `27aa40d` |
| 2026-09-30（剩余四项） | 选择性适配 `bf2027a`、`aa07f77` / `357787d` 预设快照、`08d80c3`、`1bd2ed8` / `9f91bad`；固定参考 `357787d` | 四项适配已分别提交至 `dev`；保留 fork 调度/渠道模型；运行验证及只读代码审查完成，仍有下述既有检查限制 | `3add2ca`、`c5e91a4`、`5655825`、`4e10525`；基线 `dev` @ `16d7bab` | 在 `sync/upstream-2026-09-30-remaining` 实施验证后，同基线带回 `dev`；经用户后续明确授权分别提交；未接入上游历史，连续审查点仍 `27aa40d` |
| 2026-10-01 | 完整续审 `27aa40d..0538c3e`（29 提交），其中 `357787d..0538c3e` 新增 4 提交 | 全部有明确处理结论；仅更新台账，无代码合入 | —；审查基线 `dev` @ `8b1dc92` | 新增三项功能可手工适配，版本注释不引入；旧范围能力覆盖/取舍及实施候选见下节；连续审查点推进至 `0538c3e`，不接入上游历史 |
| 2026-10-01（三项功能） | 手工适配 `0538c3e`、`8e93fa6`、`bdc9948` | 三项已实施、实测并经用户授权分别提交至 `dev` | `8b1110a`、`5156f9c`、`3afa382`；基线 `dev` @ `d484661` | 在 `sync/upstream-2026-10-01-three-features` 实施并同基线带回 `dev`；不 merge/cherry-pick 上游历史，不改变调度/schema/认证隔离，连续审查点仍 `0538c3e` |

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



## 2026-10-01 连续审查：`27aa40d..0538c3e`

实际执行 `git fetch upstream --prune`，上游从旧快照 `357787d` 前进至 `0538c3e`；`git rev-list --count` 确认旧连续审查点之后 29 提交、旧快照之后 4 提交。三个只读切片逐补丁比对本地实现，主代理以本地 Git 对象复核新提交和关键边界提交，并核验下述源码与 LSP 引用。本轮没有修改业务代码、更新依赖、cherry-pick、merge、提交或推送；没有运行测试、构建、服务或浏览器，以下是静态兼容性判断，不是功能验证通过声明。

“适配候选（未引入）”表示审查判断已完成，但功能未实施；不计入“待审查提交数量”，也不表示已获得实施授权。

| 上游提交 | 更新内容 | 当前处理结论 |
|---|---|---|
| `bab1a53` | 日志来源 API Key 名称 | 功能覆盖；fork 的 `RequestAPIKeyName` / `APIKeyName` 已贯穿登记、终态日志与卡片/详情，非原补丁引入 |
| `edb4bfb` | 来源 Key 与指标网格样式 | 不引入中间态布局；保留 fork 响应式指标与 Key 展示 |
| `bf2027a` | 轮次开始时刻与实时状态样式 | 部分引入，开始时刻由 `3add2ca` 适配；不采用 RequestState 客户端推断状态机或 spinner 配色 |
| `7a720e9` | v0.13.3 注释 | 不引入；fork 无该版本注释机制 |
| `1c48ee5` | 请求头 client_header | 已由 `eaa5a12` 适配，保留原始入站快照与认证头优先 |
| `0b919da` | 禁用渠道不参与选路 | 功能覆盖；fork 在 `GroupGetEnabledMap` 与真实尝试前过滤/跳过，不采用 Available / ChannelGrant 结构 |
| `9a80de3` | 日志 spinner 移除绿色 | 当前样式已覆盖；不另引入上游布局补丁 |
| `1bd2ed8` | Go 依赖升级 | 四项直接依赖及必要闭包已由 `4e10525` 选择性适配；不复制全部上游依赖树，axonhub 由 CI unstable 解析 |
| `6dce286` | v0.13.4 注释 | 不引入 |
| `d5a893f` | 手动获取模型的全局过滤 | 已由 `0a4482b` 适配；自动同步与分组删除路径不变 |
| `08d80c3` | 三路径 Logo | 已由 `5655825` 适配几何与缓存刷新；保留 fork 动效与 reduced-motion |
| `00e0739` | 实时输出计数/速度首版 | 不引入该计数链路；初版解析字符，但后续最终形态变为 SSE 事件数，不是真实 token/字符数 |
| `44f63cb` | 手动取消整个请求 | 适配候选（未引入）；与既有停止单次尝试不同，需要请求级取消贯穿在途调用、后续重试及终态，不能搬 RequestState 或改名既有路由 |
| `a0a5c66` | 首字、速度、缓存率与事件估量 | 首字/终态 tokens/s/缓存率功能覆盖；不引入每事件估 2 字符、响应分时状态机或上游布局 |
| `6f330f5` | v0.13.5 注释 | 不引入 |
| `d18a419` | 每事件计 1，成功状态分支调整 | 不引入；计数仍非真实字符/token；不迁移 RequestState 成功/取消处理 |
| `4a54b59` | v0.13.6 注释 | 不引入 |
| `aa07f77` | 修正服务商预设 | 已由 `c5e91a4` 按单主协议适配，不引入 dialect/schema |
| `0e1c3fc` | v0.13.7 注释 | 不引入 |
| `276ec89` | 创建渠道失败 toast | 功能覆盖；fork `Create.tsx` 已显示本地化标题与错误描述 |
| `758b582` | axonhub 伪版本升级 | 不单独引入；补丁仅更新 axonhub，CI 既有 unstable 解析机制覆盖，不等于本地已安装上游伪版本 |
| `ecb4923` | 思考等级、指标布局、重试错误保留 | 思考等级/首字/缓存率/终态速度功能覆盖；不采用布局。重试期间保留最近错误为可选适配候选（未引入），可从既有 attempts 派生，无需新增状态机 |
| `9357a32` | v0.13.8 注释 | 不引入 |
| `9f91bad` | Go 依赖升级 | 四项直接依赖及必要闭包由 `4e10525` 选择性适配；保留两项 replace，不复制其余直接/间接版本 |
| `357787d` | 扩充至 17 项服务商预设 | 已由 `c5e91a4` 适配；仅 type/base_urls 预填与三语说明 |
| `8e93fa6` | 日志展示所用渠道 Key 名称 | 已由 `5156f9c` 手工适配；独立 channel_key_id + channel_key_remark 当次快照，五处文本展示，不拼入 channel_name、不记录密钥正文 |
| `a94a740` | v0.13.9 注释 | 不引入；不是运行时功能或修复 |
| `bdc9948` | 反向代理子路径支持 | 已由 `3afa382` 手工适配；统一 API/SSE/导入导出/SW URL，shell/static 与强刷新按目录隔离、字体共享；代理仍须剥前缀并规范末尾斜杠 |
| `0538c3e` | 分组编辑成员置顶/置底 | 已由 `8b1110a` 手工适配；创建/编辑共用首尾草稿按钮，保存沿用 priority；列表卡片继续仅拖拽，不改后端/schema/调度 |

### 审查时的适配顺序与关键边界

以下保留实施前的定位与判断；三项最终实现和运行证据见下节，不把历史审查状态当作当前功能缺口。

1. **优先分组编辑置顶/置底（`0538c3e`）**：`ItemList.tsx:24` 已有重排算法，`Editor.tsx:223` 已消费 `onReorder`，`internal/op/group.go:130` 已支持 priority 更新。上游以 `!group` 限定编辑态，fork 没有该 prop，应明确区分编辑器与列表卡片；后者 `Card.tsx:351` 的 `onReorder=setMembers` 仅更新本地，持久化通过 `onDrop=handleDropReorder`，不能让按钮只改本地顺序。按钮须沿用现有组件、三语与可访问名称，不扩展排序/调度语义。
2. **日志渠道 Key 标识（`8e93fa6`）**：`model.ChannelAttempt.ChannelKeyID` 经 iterator、`op/log_store.go:147` 和前端 `normalizeAttempt` 已完整透传；LSP 确认前端字段仅有归一化与接口引用，没有 UI 消费。fork Key 模型只有 `Remark`，没有上游 `Name`。采用独立 ID/备注呈现，保留 `ChannelName`；`internal/op/stats.go:877,1040` 按 channel_id/channel_name 分组/去重，拼接 Key 会污染统计维度。绝不使用 `ChannelKey` 明文作为名称。
3. **有子目录部署需求再适配 `bdc9948`**：`vite.config.ts:7`、manifest 的 start_url/scope 已相对化；缺口是 `api/client.ts:76`、`api/setting.ts:147,176`、`api/endpoints/log.ts:530,732,973` 的三条 SSE、`sw-register.tsx:30`、`public/sw.js` 的根路径壳/资源/旁路规则和缓存清理，以及 `lib/sw.ts` / `setting/Info.tsx`。必须保留字体缓存既有契约、限定当前应用 scope 的缓存/注册清理，不能覆盖 fork SW 为上游简化版本。后端当前仍根路径挂载，需代理剥离前缀；需验证根路径回归、带末尾斜杠的子路径登录/API/SSE/导入导出、刷新/离线、同源双实例不互删缓存。
4. **旧范围可选项**：`44f63cb` 整请求取消要按 fork 的 `relayRun` 与 log store 适配，定义取消与成功/已提交流的终态边界，保留单次尝试停止和现有正文路由；`ecb4923` 的重试错误保留可纯前端从 attempts 派生，目前重试 tooltip/详情已有错误，不是错误数据缺失。

没有推荐原样 cherry-pick 的功能提交，也不建议整体 merge 上游 master。已覆盖的能力与纯版本注释不重复合入；上游实时事件计数链路明确不采用。前三项已按下节完成手工适配；`44f63cb` 整请求取消与 `ecb4923` 重试错误保留仍未实施，不混入本批。

### 2026-10-01 三项功能适配与验证

- 固定来源：`0538c3e715e529e3a1f3a2b1229addb8ca4357ea`、`8e93fa6fbc90383f765f32bf9a611604eb6a191b`、`bdc99486b0b6f400633dcaeedf415927c9805ff8`。从本地 `dev` @ `d484661` 建临时 sync 分支实施并同基线带回，保留既有零占位覆盖滚动条和原台账文本；未 fetch 扩大来源、未合并上游历史。后续经用户明确授权，三个功能分别提交为 `8b1110a`、`5156f9c`、`3afa382`，验证限制保持如下。
- 分组真实浏览器/API：创建与编辑首尾操作只改草稿、点击无 create/update；保存后回读优先级与重开顺序一致；取消不写入，受控 500 更新失败保留草稿后恢复保存。空/单成员、首末禁用、删除中和拖拽 clone 禁用、Enter/Space 不提前提交通过；列表卡片无新按钮，原键盘 DnD 真实持久化，成员数与权重不变。
- 渠道 Key 真实 relay：fixture 503→另一 Key 成功的两次出站已观察，overview attempts/history、detail started/finished/终态及列表的 ID/trim 备注一致。渠道名不变，元数据无密钥正文；改备注后旧记录同进程刷新仍保持快照，新记录显示新备注。空备注仅 ID、真实 Key 级熔断仅 ID、无可用 Key 的渠道级跳过无 ID；PRAGMA 确认无新增 remark 列。
- 页面与兼容消费：主卡片、重试 tooltip、Live 历史、选中尝试头和旧协议历史五处已验证；运行态人工选择失败尝试后不被成功终态覆盖。旧列表协议及缺 ID/备注采用捕获真实记录的受控响应验证，不冒充真实 relay 生命周期。461 字符备注（420 连续 ASCII 加脚本文字）作为普通文本换行，未执行脚本；短屏重试列表限高、真实 wheel/轨道滚到底、离开正常关闭。
- 根路径和 `/octopus/` 的生产嵌入页面：登录/刷新/渠道分组读取、日志详情正文、真实导出下载及同文件 multipart 导入成功；导出参数和文件名保留，非法 JSON 保持原错误。页面凭据退出 API 200 后状态接口 401。三条 SSE 分别直连读取首帧后关闭；子目录原始 API/SSE/备份 URL 均保留前缀，代理剥成后端根路由；308 保留页面 query。
- 真实 PWA：同一独立 Chromium profile 安装 `/`、`/octopus/`、`/second/` 三 scope 与编码目录缓存。子目录先安装保留旧 root v3，root 激活只移除精确旧 root shell/static；字体/无关 marker 保留。两子目录离线 HTML、哈希 JS/CSS 和 Logo 成功，未缓存 API 失败；各目录强刷新只清自身非字体缓存并只注销对应 scope，无匹配子注册时不注销 root；worker CLEAR_CACHE 通知及目录隔离实测通过。
- 检查：`go test ./... -skip '^TestCompactSQLite$'` 通过，明确跳过已报告的 Windows 目录 sync 用例；`pnpm build`（TypeScript + Vite）及 `node --check public/sw.js` 通过。10 个业务修改 TS 源 ESLint 无诊断；必要关联改动的 tooltip primitive 仍有既有 set-state-in-effect / context 导出 3 个诊断，未抑制或顺手重构，不称全仓 lint 通过。
- UI 实测：1280×900 简体/亮色、390×844 英文/暗色、390×400 繁体/亮色，无新增横向溢出，保存及详情操作可经内部滚动/焦点访问，原覆盖轨道零占位保持。仅 Chromium 和 CDP 模拟触摸，不声称真机、Safari 或 Firefox 已验证。
- 运行边界与部署要求见 `web/README.md`：日志是进程内快照，不新增重启历史持久化；生产代理剥前缀、入口补斜杠、SSE 不缓冲；同 origin 的 cookie/localStorage 仍共享，不承诺认证隔离。


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
