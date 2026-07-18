## Target

为渠道倍率增加可信的自动同步来源状态，并在渠道卡片中区分渠道自动同步与倍率自动同步。

## Dependents (6)

- `internal/model/channel.go`: 渠道持久化模型和 API 响应。
- `internal/op/channel.go`: 渠道手动更新与缓存刷新。
- `internal/server/handlers/channel.go`: 渠道创建入口。
- `internal/task/sync.go`: 从上游获取并保存倍率。
- `web/src/api/endpoints/channel.ts`: 前端渠道数据契约。
- `web/src/components/modules/channel/Card.tsx`: 渠道名称、倍率和同步状态展示。

## Affected Stories

- 当前仓库没有 `specs/release-plan.yaml` 或 epic capsule；本次属于渠道倍率展示行为修正。

## Test Coverage

- `internal/helper/fetch_test.go`: 已覆盖上游倍率获取成功及不支持查询。
- `internal/task/sync_test.go`: 已覆盖模型同步的空结果保护，但未覆盖倍率来源状态。
- 新增 `internal/op/channel_test.go`: 覆盖手动修改清除来源状态、内部同步设置来源状态。
- 前端没有组件测试；通过 TypeScript、ESLint 和诊断检查验证数据契约与 JSX。

## Risk: Medium

变更跨越数据库模型、更新语义和前端 API，但新增字段默认值兼容已有数据，且不影响倍率参与调度的数值逻辑。

## Recommended action

继续实施；成功获取倍率时设置来源状态，手动修改倍率时清除状态，同步失败时保留当前倍率及来源状态。
