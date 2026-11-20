# relay-forge

基于 [new-api](https://github.com/QuantumNous/new-api)（AGPL-3.0）的分支，面向做 LLM API 中转站的站长。
上游的鉴权、渠道调度、计费和原子扣费逻辑都保留，这里只**增加**三块上游没有的能力（分销抽成是在扣费成功之后追加一次调用，不改变扣费本身）。

> 状态：MVP。三块都只有后端 API，还没有前端页面。

## 新增能力

### 1. 站长收益看板（只读，管理员）

`GET /api/admin/earnings/overview`　总览
`GET /api/admin/earnings/by-channel`　按渠道
`GET /api/admin/earnings/by-model`　按模型

参数：`start_date`、`end_date`（`YYYY-MM-DD`，结束日含整天，默认最近 30 天）；分组接口另有 `p`、`page_size`（有上限，不会返回无界列表）。

返回：`total_revenue`（收费减退款）、`total_cost`、`gross_profit`、`gross_margin`、`request_count`、`refund_quota`，以及 `cost_source`、`cost_ratio`、`date_range`。

**成本口径（重要）**：日志表不记录真实上游成本，目前也没有任何代码去写它。所以成本 = 收费 ÷ 1.2（按默认加价倍率倒推），`cost_source` 为 `estimated`。后果是每个渠道、每个模型的毛利率都是同一个值，**这里的"利润"只是示意，不是真实利润**。日志表一旦有 `cost_quota` 列，会自动改读真实值（`cost_source` 变为 `recorded`）。真正按渠道算利润，需要先记录每个渠道的进价倍率，这是下一步。

### 2. 渠道健康巡检

- 每 30 分钟一轮，只在主节点跑；启动后第一轮在一个周期之后。管理员可 `POST /api/channel/probe` 立即扫一轮（返回 202 和 `started`）。
- 只探测 OpenAI 兼容和 Anthropic 渠道，其余类型跳过。每个渠道最多探测 5 个账号（key），发一条 `hi`（`max_tokens=5`），模型取渠道的测试模型，否则取渠道里最便宜的聊天模型。
- 判定：`healthy`；`degraded`（HTTP 200 但内容为空、是致歉套话或无法解析，即降智/被限流的典型表现）；`failed`（非 200 或网络错误）。
- **渠道状态只在站长授权的范围内改变**，规则和 new-api 对真实请求、渠道测试用的是同一套：
  - 停用：需要全局"自动禁用渠道"开关打开，且命中配置的状态码（默认 401）或关键词（余额不足、组织被禁用等）。429、5xx、超时只计数、记日志，**不停用**，因为一次探测分不清它们是上游忙还是真故障。
  - 降智：新状态 `degraded`（4），同样需要全局"自动禁用渠道"开关。
  - 渠道自己关掉 `auto_ban` 的，既不会被探测停用，也不会被标降智。
  - 恢复：需要全局"自动启用渠道"开关；手动停用的渠道永远不动。
  - 这两个全局开关出厂都是关的。**不打开时，巡检只记录响应时间、失败次数和日志，不改路由。**
- 探测机自己断网（同一代理后至少 3 个账号全无响应）时，本轮不计入失败，也不停用。
- 渠道新增只读字段 `error_count`：探测失败累计数，探测通过后清零。

### 3. 分销持续抽成

- 用户新增 `commission_rate`（0–1），记在**邀请人**身上。管理员用 `PATCH /api/user/:id/commission`，body `{"commission_rate": 0.1}` 设置，会写审计日志。
- 下级每次实际消费 `quota`，邀请人钱包入账 `floor(quota × 比例)`，并给邀请人写一条系统日志。没有邀请人、比例为 0、邀请人已删除都不抽成。预扣结算和直接扣费两条路径都覆盖，且互斥，不会重复入账。
- 与上游原有的注册一次性邀请奖励（`aff_quota`）互不影响。
- **运营提醒**：抽成入账的是可直接消费的额度，成本由站长自己承担。比例要低于你的毛利率，否则自己注册小号互相邀请就等于打折，比例接近 1 就等于免费用。

## 构建与运行

和上游一致，见 [README.md](./README.md)。`make build-web`（需要 bun）生成 `web/dist` 之后再 `go build`；只想编译后端时，放一个占位的 `web/dist/index.html` 即可。

新增的 admin 路由要登记到 `middleware/access_token_routes.go`，新增的渠道字段要归类到 `controller/channel_authz.go`，否则 `router`、`controller` 里的守卫测试会失败。

## 与上游的关系

- 根提交 `cc19772` 是 new-api 在 `48f9df9` 的源码快照（不含历史）；第二个提交把 Go module 改名为 `github.com/mazihua-lgtm/relay-forge`；之后是本分支的改动。
- 同步上游的思路：取 `48f9df9..upstream/main` 的差异，把补丁里的模块路径换成本仓库的，应用后 `gofmt -w`（改名会让 import 的排序变化）。**这套流程还没实操过。**
- 许可证 AGPL-3.0（见 `LICENSE`），上游版权声明保留。AGPL §13 要求：以网络服务形式向用户提供修改过的版本时，需要向这些用户提供对应源码。做中转站的站长务必留意。
