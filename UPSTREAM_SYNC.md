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
| 当前上游最新提交 | `27aa40dc0f3b2902bce3e96ccdba019d17041606` (`v0.13.2`) |
| 本次已审查至 | `27aa40dc0f3b2902bce3e96ccdba019d17041606` |
| 本次审查所在本地基线 | `dev` @ `3d606b01a7c06dad27c3246e2fe69afd92073212` |
| 本次新增上游代码合入 | 无；本次只审查，没有 cherry-pick 或 merge |
| 待审查提交数量 | 0 |

## 已审查提交

范围：`dddc6bc..upstream/master`。以下结论针对当前 `dev` 文件树；“等价补丁”表示补丁内容与本地提交相同，“功能覆盖/功能适配/独立实现”表示功能已存在但原始上游 SHA 和补丁均未进入 Git 历史。

| 上游提交 | 处理结果 | 本地提交 | 备注 |
|---|---|---|---|
| `6e736b4` | 独立实现（原提交未引入） | `ae3d0c5`, `3d606b0` | 当前 fork 早已有 `Channel.Keys` 与 `channel_keys`，并保留自己的 key 选择逻辑；未引入上游 `ChannelGrant`、迁移 011/012 及协议/授权模型 |
| `1be0acd` | 等价引入 | `de36a05` | `git cherry -v dev upstream/master` 标记为 `-`；两者 patch-id 相同，均移除 auth middleware 中相同的 6 行前缀校验 |
| `1ab744f` | 功能覆盖（非原补丁） | `12cd0be` | 当前 `setupLogSSE` 在日志快照前写入 `: connected` 并 Flush，覆盖空日志流即时刷新；实现早于该上游提交且不是同一 patch |
| `f07b8dc` | 未引入 | — | 仅把 `main.go` 版本注释改为 `v0.13.0`；当前 `main.go` 没有该版本注释 |
| `13a144a` | 不引入 | — | 按项目决定不引入 Issue 模板 AI 撰写确认项和 `template-check.yaml`；属于仓库治理，不影响运行时产品能力 |
| `3364bb1` | 未引入 | — | 上游依赖批量升级未采用；当前 `go.mod` 与 `web/package.json` 版本仍是 fork 自己的版本集合 |
| `e3af162` | 功能适配（非原补丁） | `1d1e722` | 当前静态中间件和 Vite 构建配置均支持 gzip；本地版本另外实现 q 值/wildcard 协商与 406，patch-id 不同 |
| `dd21e66` | 未引入 | — | 上游只修复 `internal/db/migrate/005.go`；当前 fork 迁移目录只有 001–004，目标迁移/schema 不在当前架构中 |
| `7bbf77d` | 未引入（仅有独立同类改动） | — | 上游 23 文件优化依赖其 ChannelGrant/日志结构；当前没有 `TargetItemID`、`dismissOnClickOutside` 等对应改动，局部取消按钮/弹窗行为来自 fork 自己的历史提交 |
| `27aa40d` | 未引入 | — | 仅把 `main.go` 版本注释从 `v0.13.1` 改为 `v0.13.2`；当前没有该注释 |

## 同步批次记录

| 日期 | 上游范围 | 结果 | 本地合并/提交 | 备注 |
|---|---|---|---|---|
| 本轮审查 | `dddc6bc..27aa40d` | 10 个提交均已判定；未新增代码合入 | `dev` @ `3d606b0` | 后续从 `27aa40d` 之后的上游提交开始评估 |


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
