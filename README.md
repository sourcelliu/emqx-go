# emqx-go
golang implementation of  EMQX 

参考 `https://github.com/emqx/emqx`原始项目，希望在turtacn/emqx-go 项目中完成下面的任务

任务标题：**全量重写 — emqx (Erlang/OTP) → emqx-go (Go)**

目标简介：

把 `https://github.com/emqx/emqx` 的核心功能（MQTT broker：connect/subscribe/publish/session/cluster）**全部重写为 Go**，将代码推入 `https://github.com/turtacn/emqx-go` 仓库。产出需包含可运行的 PoC、完整的迁移计划、测试套件、CI/CD、Kubernetes 部署与运维文档。**这是一次全量重写（full rewrite）任务，不是逐步替换。**

---

## 一、总体要求（必须严格遵守）

1. 用 Go（至少 go1.20+）实现，代码遵循 gofmt、golangci-lint（配置放在 `.golangci.yml`）。
2. 按阶段输出（Phase 0..N），每个阶段有明确目标、交付物、验收条件。
3. 所有关键输出都要可执行（PoC 能运行并通过 CI 测试）。
4. 代码提交到 `turtacn/emqx-go`，每个阶段产生单独分支与 PR（branch 命名规则：`phase/{n}-{short-desc}`）。
5. 对比与映射表格（Erlang 模块 → Go 模块），以 CSV/Markdown 表格形式提交。
6. 提交完整文档（README、设计文档、运维手册、回滚手册）到仓库 `docs/` 目录内。
7. 在每个 PR 中包含变更摘要、影响分析、性能预估与回退步骤。

---

## 二、阶段划分（Phase & 交付物与验收）

### Phase 0 — 探索与基线（Deliverables）

* 任务：静态分析 emqx 源码，生成模块清单与依赖图。
* 输出：`/docs/phase-0/` 包含：

  * `modules.csv`（列：erl\_module, description, complexity(H/M/L), depends\_on, n\_lines）
  * `baseline_metrics.md`（当前 Erlang 在测试环境下的吞吐/延迟/99p 基线，若无法运行则给出估算指标和获取步骤）
  * `mapping_table.md`（初步 Erlang → Go 模块映射）
* 验收：提交 PR `phase/0-discovery`；PR 必须包括模块表，且 CI 检查（静态脚本）通过。

### Phase 1 — mini-OTP 运行时 PoC（核心：supervisor + actor）

* 任务：实现一个最小可用的 `mini-OTP` 库（`/supervisor`），提供：

  * actor/mailbox 抽象（带缓冲、可选 selective-receive）
  * supervisor 支持 `one_for_one`、restart backoff、health check API
  * 简单监控 hook（Prometheus metrics）与日志接口
* 输出：`/supervisor` 模块（带 README、单元测试覆盖） + 示例 service 使用示例 (`/examples/echo`)
* 验收：`go test ./...` 全通过；示例能启动并演示 child crash → 自动 restart；PR `phase/1-mini-otp`。

### Phase 2 — MQTT 基础协议 PoC（broker minimal）

* 任务：基于 Phase1，实现最小 MQTT Broker 功能（CONNECT、SUBSCRIBE、PUBLISH to local subscribers），支持单节点：

  * `broker/`、`session/`、`storage/`（接口）
  * 提供 MQTT 协议兼容测试（契约测试），并能与原 emqx 在基本场景下行为一致
* 输出：可启动的 broker demo（在本地监听 1883），带单元与契约测试、README、dockerfile、k8s manifest（简单 Pod）
* 验收：能成功用 `mosquitto_pub/sub` 与 PoC broker 互通；契约测试通过；PR `phase/2-broker-poc`。

### Phase 3 — 集群与路由 PoC（两节点）

* 任务：实现 cluster 模块（gRPC 或 NATS）实现订阅路由共享、会话迁移/replication demo：

  * cluster discovery（etcd 或 k8s）
  * 简单路由同步协议（gRPC streaming）
* 输出：两节点 demo（docker-compose 或 k8s），演示在节点 A 上订阅，节点 B 能收到 publish（跨节点转发）；带压力测试脚本（target: tens of thousands connections 在 PoC 环境）。
* 验收：两节点 demo 在本地/测试集群启动并互通；路由表一致性在 N 秒内收敛；PR `phase/3-cluster`。

### Phase 4 — 数据迁移设计与脚本

* 任务：设计 Mnesia/ETS → Postgres/Redis 的完整迁移方案，并提供迁移脚本 PoC：

  * schema 映射、双写实现示例、并行校验工具（checksum 对比）
* 输出：`/migrations` 包含迁移脚本（Go）、迁移步骤文档、回滚脚本
* 验收：能从 sample Mnesia dump 转换并导入 Postgres，且校验工具返回一致性 OK；PR `phase/4-migration`。

### Phase 5 — CI/CD、监控、压力与混沌测试

* 任务：构建 GitHub Actions（或 GitLab CI）流水线，包含：lint/test/build/docker-push/integration/contract/load-test；实现 Prometheus metrics 与 Grafana dashboard JSON；混沌测试脚本（网络分区、node kill）。
* 输出：`.github/workflows/ci.yml`、`charts/emqx-go`、`monitoring/grafana-dashboard.json`、`tests/load/`、`tests/chaos/`
* 验收：CI 在 PR 上跑通（lint+unit+contract），压力测试能生成基线报告，混沌脚本可复现并记录行为；PR `phase/5-ci-monitoring`.

### Phase 6 — 生产化建议与最终报告

* 输出：`/docs/final-migration-plan.md`（含风险矩阵、SLO、切换策略、回滚 plan、人员与时间建议）
* 验收：最终 PR 包含完整交付物并通过仓库管理员 review。

---

## 三、agent 执行细则（必须执行的自动化步骤与命令）

> 在执行任何写入 `turtacn/emqx-go` 仓库操作前，先开启 dry-run 模式并提交 discovery 报告。只有在 Phase 1 验收后允许写入实际目录的代码。

1. 克隆仓库：

```bash
git clone https://github.com/emqx/emqx.git /workspace/emqx-src
git clone https://github.com/turtacn/emqx-go.git /workspace/emqx-go
cd /workspace/emqx-go
```

2. 分支规范（示例）：

```bash
git checkout -b phase/1-mini-otp
# 完成后
git add .
git commit -m "phase1: add mini-OTP supervisor & actor PoC"
git push origin phase/1-mini-otp
# 创建 PR：标题包括 [phase/1] mini-OTP PoC
```

3. 本地运行测试：

```bash
# 运行 go test
GOFLAGS="-mod=mod" go test ./... -v
# 运行示例
go run examples/echo/main.go
```

4. Docker / K8s：

```bash
# build
docker build -t registry.example.com/turtacn/emqx-go:phase1 .
# push
docker push registry.example.com/turtacn/emqx-go:phase1
# deploy to k8s (测试环境)
kubectl apply -f k8s/phase1-deployment.yaml --context=testing
```

5. 契约测试（示例）：

* 为 MQTT 协议写契约测试：模拟 emqx/emqx 的 CONNECT/SUBSCRIBE/PUBLISH 响应并断言行为一致。

6. 压力测试（示例）：

```bash
# 使用 k6/vegeta
k6 run tests/load/mqtt_connect.js
vegeta attack -targets=targets.txt -rate=1000 -duration=30s | vegeta report
```

7. 监控与回滚：

* 部署 Prometheus scrape config，收集 metrics（`emqx_go_process_restarts_total`, `emqx_go_mailbox_depth` 等）。
* 回滚：在任何阶段若 SLO 严重下降，立即停止流量并回退到上一个镜像标签（`kubectl set image deployment/...`）。每次切换必须记录 snapshot（metrics + logs + traces）。

---

## 四、交付物格式约定（便于 agent 自动生成）

* 代码：`/pkg/...` 按模块分包；主命令在 `/cmd/emqx-go/main.go`。
* 文档：Markdown，放 `docs/phase-x/`。
* Diagrams：Mermaid 格式文件放 `docs/diagrams/*.mmd`（agent 请同时生成 PNG）。
* 测试报告：JSON + HTML 报告放 `tests/reports/phase-x/`。
* CI：`.github/workflows/ci.yml`。
* Helm：`charts/emqx-go/`。

---

## 五、验收标准（每个阶段必须满足）

1. 所有 Go 代码通过 `golangci-lint`、`go vet`、`go test`（单元覆盖率 >= 70% 对关键模块）。
2. Phase2 的 broker PoC 能用 mosquitto 客户端互通（connect/publish/subscribe）。
3. Phase3 cluster demo 在两节点下完成路由同步且在 5 秒内收敛（在 PoC 环境下）。
4. 数据迁移脚本能对 sample 数据集合进行导入并通过 checksum 验证。
5. CI pipeline 在每个 PR 自动运行并成功（lint/test/contract）。
6. 每个阶段的 PR 包含迁移影响分析与回滚步骤。

---

## 六、风险矩阵（示例）与缓解措施（agent 必须在每个阶段报告）

* 风险：丢失 OTP “let it crash” 容错语义。

  * 缓解：在 Phase1 实现 supervisor/restart/backoff，Phase2 开始逐步用 contract tests 验证 failure semantics。
* 风险：Go GC 导致突发延迟。

  * 缓解：优化内存分配，测试不同 GOGC 值，使用 sync.Pool，关键路径使用 bounded worker pool。
* 风险：消息顺序/幂等性问题。

  * 缓解：引入持久化消息队列（Kafka/NATS JetStream），实现幂等处理与消息序列号。
* 风险：数据一致性失败。

  * 缓解：双写策略 + 校验工具 + 回滚脚本。

agent 在每个阶段的 PR must include 一个 `RISK.md` 总结表。

---

## 七、示例任务片段（便于 agent 并行工作）

* 任务 A（工程师 agent）：实现 `/supervisor` 包并覆盖单元测试。输出 PR `phase/1-mini-otp`。
* 任务 B（测试 agent）：编写 MQTT 协议契约测试并验证与 emqx/emqx 在基本场景的兼容性。输出 `tests/contracts/`。
* 任务 C（infra agent）：生成 Dockerfile、k8s manifest 与 Helm Chart，验证能在测试 k8s 集群部署 PoC。

每个子 agent 必须在完成后在主 repo 创建对应的 PR，并在 PR 描述中写明“已完成的验收条件 + 测试结果 + 遗留问题”。

---

## 八、最终交付（完成后必须提交）

1. `turtacn/emqx-go` 主分支：合并所有 Phase 的 PR（或按组织策略合并到 release 分支）。
2. `docs/final-migration-plan.md`：包含详细迁移窗口、人员安排、切换脚本（自动化）与回滚脚本。
3. `reports/`：所有压力测试、契约测试、混沌测试报告。
4. `charts/emqx-go/`：用于 k8s 一键部署 PoC。

---

## 九、附：示例 Mapping 小表（agent 需以此为模板扩展成完整表格）

（agent 在 Phase0 必须扩展为完整 CSV）

| Erlang/OTP 概念    |                       emqx 核心模块 示例 | Go 目标模块                                      | 备注                               |
| ---------------- | ---------------------------------: | -------------------------------------------- | -------------------------------- |
| gen\_server      |             `emqx_server` variants | `pkg/supervisor/actor` + `pkg/broker/server` | actor 模拟，带 mailbox               |
| supervisor       |               OTP Supervisor trees | `pkg/supervisor`                             | 实现 one\_for\_one, restart policy |
| mnesia/ETS       | session store, subscription tables | Postgres (meta), Redis (cache)               | 视访问模式拆分                          |
| distribution     |             node-to-node messaging | gRPC streaming + etcd service discovery      | 明确网络边界                           |
| hot code upgrade |                       code\_change | rolling deploy + feature flags               | 需替代方案                            |

---

## 十、agent 必须的交互/报告频率

* 每完成一个 Phase，agent 在仓库创建 PR，并自动在 `docs/phase-x/report.md` 写入阶段总结（包含：已完成项、未完成项、测试结果、性能指标、风险与缓解）。
* 若任何阶段测试失败或 SLO 降低超过阈值（预设阈值由你定义，例如 error rate > 0.1% 或 p99 延迟翻倍），agent 必须暂停进一步自动合并并通知人工干预（open issue + assign）。

---

# 结束语（如何使用）

1. 把上面整个内容原样粘贴到你的 agent 控制台/任务输入框（或保存为任务模板）。
2. 在 agent 配置中填上凭据（GitHub token、Docker registry、k8s context）。
3. 先设置 `dry-run=true` 执行 Phase 0，review Phase0 output；人工确认后允许 agent 继续 Phase1+。

