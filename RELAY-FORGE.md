# relay-forge

基于 [new-api](https://github.com/QuantumNous/new-api)（AGPL-3.0）的分支，面向做 LLM API 中转站的站长。
上游的鉴权、渠道调度、计费和原子扣费逻辑都保留，这里只**增加**三块上游没有的能力（分销抽成是在扣费成功之后追加一次调用，不改变扣费本身）。

> 状态：MVP。三块都有管理后台页面（见各节的「后台入口」），API 也可以单独调用。

## 新增能力

### 1. 站长收益看板（只读，管理员）

**后台入口**：侧边栏管理区的「收益 / Earnings」（`/earnings`，仅管理员）。顶部是时间范围（最近 7 天 / 30 天 / 本月 / 上月，或自选起止日期），下面是收入、上游成本、毛利（含毛利率）、请求数四张卡片和「按渠道 / 按模型」两张分页表；成本旁边的标签说明它是「已记录 / 估算 / 混合」，有估算成分时页面顶部会提示，并带一个去渠道页设置进价倍率的按钮。手机上也能看全（退款和毛利率不会被折叠）。

`GET /api/admin/earnings/overview`　总览
`GET /api/admin/earnings/by-channel`　按渠道
`GET /api/admin/earnings/by-model`　按模型

参数：`start_date`、`end_date`（`YYYY-MM-DD`，结束日含整天，默认最近 30 天）；分组接口另有 `p`、`page_size`（有上限，不会返回无界列表）。

返回：`total_revenue`（收费减退款）、`total_cost`、`gross_profit`、`gross_margin`、`request_count`、`refund_quota`、`cost_source`，以及 `cost_ratio`、`date_range`。总览、每个渠道、每个模型的每一行都带自己的 `cost_source`；分组接口顶层的 `cost_source` 是整个时间窗口的，不是当前这一页的。

**成本口径（重要）**：成本有两个来源，按请求分别取：

- **已记录（`recorded`）**：给渠道设置了"进价倍率"（见下）之后产生的每条消费日志，在写入的那一刻记下上游成本：`成本 = 实收额度 ÷ 分组倍率 × 渠道进价倍率`。这是快照，以后再改进价，历史报表不会被改写。
- **估算（`estimated`）**：没设进价倍率的渠道、设置之前的旧日志，按 `实收 ÷ 1.2`（默认加价倍率倒推，即响应里的 `cost_ratio`）估算。
- 同一行里两者都有时是 `mixed`：已记录的用真实值，只对"未记录"的那部分额度估算，不会把旧数据当成零成本。

所以**没给渠道设进价倍率之前，每个渠道的毛利率都是同一个估算值，不是真实利润**；设了之后，只有新产生的请求有真实成本，旧数据仍是估算。

#### 渠道进价倍率 `cost_ratio`

渠道新增字段 `cost_ratio`：这个渠道的进货价，相当于你为该模型设的**基准价（模型倍率或固定价，不含用户分组倍率）**的几倍。例：你按基准价的 4 折从上游进货，填 `0.4`；某用户所在分组倍率是 1.5，他付 1.5 倍基准价，你的成本是 0.4 倍，毛利是 1.1 倍。

- 范围 0–100；`0` 或不填表示未设置（按估算）。
- 通过渠道的新建/更新接口设置：`PUT /api/channel`，请求体里带 `"cost_ratio": 0.4`（和后台一样带上渠道的其余字段）。传 `0` 可清除；更新时不带这个字段则保持不变。
- **后台入口**：渠道编辑 →「路由与映射」→「上游成本」→「进价倍率」，填 `0` 并保存即清除（后台每次保存都会带上这个字段，所以 `0` 一定生效）。超出 0–100 时表单会跳到这一栏并报错，不会发出请求。
- 有渠道编辑权限即可修改，不需要"敏感字段"权限：它只影响报表，不影响路由。
- 算不出成本时记为"未知"（之后按估算）：渠道没设倍率；分组倍率为 0 的免费分组（看不出基准价）；日志里没有分组倍率。
- 异步任务（视频、绘图等）的计费日志同样记录成本，退款日志不记。
- 只适用于按量计费的上游。包月、订阅类渠道的成本不随用量变化，可以填一个折算后的倍率，或留空。
- 用户永远看不到成本：它不在任何日志接口的返回里，也不在日志的 `other` 字段里。

升级说明：日志表新增一列 `cost_quota`，渠道表新增 `cost_ratio`，由启动时的自动迁移完成。MySQL 8 和 PostgreSQL 加列通常是瞬时的；MySQL 5.7 等旧版本在很大的日志表上加列可能要重建表，请安排维护窗口。**ClickHouse 日志库暂不记录成本**：这一列在 ClickHouse 里会是 Nullable 类型，而日志读取会扫描表的所有列，这个组合我没有在真实的 ClickHouse 上验证过，万一扫描失败会拖垮日志页面，所以没有创建它。ClickHouse 日志库的收益看板仍按估算，日志写入不受影响。滚动升级时，比主节点先起来的节点在新列出现后最迟一分钟内发现它，不需要重启。

### 2. 渠道健康巡检

- 每 30 分钟一轮，只在主节点跑；启动后第一轮在一个周期之后。管理员可 `POST /api/channel/probe` 立即扫一轮（返回 202 和 `started`）。**后台入口**：渠道页右上角「更多操作」→「立即健康巡检」，大约一分钟后刷新页面看结果；已有一轮在跑时会提示「渠道健康巡检正在进行中」，不会重复启动。
- 只探测 OpenAI 兼容和 Anthropic 渠道，其余类型跳过。每个渠道最多探测 5 个账号（key），发一条 `hi`（`max_tokens=5`），模型取渠道的测试模型，否则取渠道里最便宜的聊天模型。
- 判定：`healthy`；`degraded`（HTTP 200 但内容为空、是致歉套话或无法解析，即降智/被限流的典型表现）；`failed`（非 200 或网络错误）。
- **渠道状态只在站长授权的范围内改变**，规则和 new-api 对真实请求、渠道测试用的是同一套：
  - 停用：需要全局"自动禁用渠道"开关打开，且命中配置的状态码（默认 401）或关键词（余额不足、组织被禁用等）。429、5xx、超时只计数、记日志，**不停用**，因为一次探测分不清它们是上游忙还是真故障。
  - 降智：新状态 `degraded`（4），同样需要全局"自动禁用渠道"开关。渠道列表里显示为「降智 / Degraded」，鼠标移上去能看到原因和时间。
  - 渠道自己关掉 `auto_ban` 的，既不会被探测停用，也不会被标降智。
  - 恢复：需要全局"自动启用渠道"开关；手动停用的渠道永远不动。
  - 这两个全局开关出厂都是关的。**不打开时，巡检只记录响应时间、失败次数和日志，不改路由。**
  - **通知**：巡检停用、标降智、恢复渠道时（多 key 渠道里是某一个 key，原因里会写 `key #N`），会发和真实请求失败、渠道测试同一套的渠道通知：邮件、Webhook、Bark、Gotify，按每个管理员自己在「个人资料 → 通知」里选的方式。只有状态真的变了才发，一个一直不通的渠道下一轮不会重复发。收件人规则沿用 new-api：管理员没选过时只有 root 收到，其他管理员要在自己的通知设置里打开「渠道通知」（Channel notifications）。
  - 巡检停用的渠道会和真实请求停用时一样，关掉它上面活跃的 WebSocket 连接；标降智只是移出调度并通知，不关连接，因为降智是个"还能回话"的软状态，探测结论万一判错也不会掐断正在用的会话。
- 探测机自己断网（同一代理后至少 3 个账号全无响应）时，本轮不计入失败，也不停用。
- 渠道新增只读字段 `error_count`：探测失败累计数，探测通过后清零。

### 3. 分销持续抽成

- 用户新增 `commission_rate`（0–1），记在**邀请人**身上。管理员用 `PATCH /api/user/:id/commission`，body `{"commission_rate": 0.1}` 设置，会写审计日志。
- 下级每次实际消费 `quota`，邀请人钱包入账 `floor(quota × 比例)`，并给邀请人写一条系统日志。没有邀请人、比例为 0、邀请人已删除都不抽成。预扣结算和直接扣费两条路径都覆盖，且互斥，不会重复入账。
- 与上游原有的注册一次性邀请奖励（`aff_quota`）互不影响。
- **后台入口**：用户列表 → 编辑 →「邀请抽成」小节，按百分比填（0–100，保留两位小数），点「保存」或按回车即生效。它自己单独保存，不用再点「保存更改」，填了没保存的草稿也不会拦住用户表单的保存。
- **运营提醒**：抽成入账的是可直接消费的额度，成本由站长自己承担。比例要低于你的毛利率，否则自己注册小号互相邀请就等于打折，比例接近 1 就等于免费用。

## 前端

新增的页面和输入框在 `web/src/features/earnings/`、渠道编辑抽屉、用户编辑抽屉里，新增文案已补齐 7 种界面语言。校验：`bun check -b`（类型）、`bun run lint`、`bun run format:check`、`bunx vitest run`、`bun run build`。其中 lint、`format:check`、`copyright:check` 在上游源码里本来就有存量失败项，这里的改动没有新增。

## 构建与运行

和上游一致，见 [README.md](./README.md)。`make build-web`（需要 bun）生成 `web/dist` 之后再 `go build`；只想编译后端时，放一个占位的 `web/dist/index.html` 即可。

新增的 admin 路由要登记到 `middleware/access_token_routes.go`，新增的渠道字段要归类到 `controller/channel_authz.go`，否则 `router`、`controller` 里的守卫测试会失败。新增日志列时要想到 ClickHouse 日志库：它的表是 `model/main.go` 里手写的建表语句，不会自动加列。像 `cost_quota` 这样可选的列，要让 `createLog` 在缺列时跳过它，免得一条消费记录因此丢失。

**测试**：根目录的 `go test ./...` **不会进入** `relaykit/`、`tokenkit/` 这两个嵌套的独立 Go 模块（上游要求它们必须能独立构建），它们要分别进目录跑：`cd relaykit && GOWORK=off go build ./... && go vet ./... && go test ./...`，`tokenkit` 同理。前端：`cd web && bun run typecheck && bun run test`。

## 与上游的关系

- 根提交 `cc19772` 是 new-api 在 `48f9df9` 的源码快照（不含历史）；第二个提交把 Go module 改名为 `github.com/mazihua-lgtm/relay-forge`；之后是本分支的改动。
- 上游的新提交通过 `upstream-sync` 分支并进来，方法见下一节。已同步到上游 `8542c9d`（2026-11-20）。
- 许可证 AGPL-3.0（见 `LICENSE`），上游版权声明保留。AGPL §13 要求：以网络服务形式向用户提供修改过的版本时，需要向这些用户提供对应源码。做中转站的站长务必留意。

## 同步上游

上游远端叫 `upstream`（推送地址已设成无效值，不会误推）。它是本机的 git 配置，不在仓库里；换一台机器重新克隆后要自己加：`git remote add upstream https://github.com/QuantumNous/new-api.git && git remote set-url --push upstream DISABLED && git config remote.upstream.tagOpt --no-tags`。同步靠一个「上游分支」`upstream-sync`：它只装**改过模块名的上游原样代码**，每次同步在它上面加一个提交，再并进 main。这样 git 能做真正的三方合并，冲突只会出现在 relay-forge 自己改过的地方。

上游分支和上游原样代码只差两处机械改动，由 `scripts/upstream-sync/rewrite.py` 完成：

1. 所有 `.go` 文件和 `go.mod` 里的 `github.com/QuantumNous/new-api` 换成 `github.com/mazihua-lgtm/relay-forge`。**包括 `relaykit/`、`tokenkit/` 两个嵌套模块自己的 `go.mod`**（当初漏了它们，导致这两个模块不能单独构建）。`.github`、Dockerfile、README 里指向上游项目的地址不动。
2. 对这些 Go 文件 `gofmt -w`（换名会让 import 的排序变化）。

main 比 `upstream-sync` 多出来的只有 relay-forge 自己的东西：新功能，少量手改的品牌文字（`main.go`、`common/init.go`），以及 Dockerfile 和发布流程里的版本号注入路径。随时可以用 `git diff --stat upstream-sync main` 检查：多出来的文件应当恰好是自己动过的文件，别的都是流程出了问题（2026-11-20 就是这样查出了嵌套模块的 `go.mod` 和一个漏了 gofmt 的测试文件）。

每个上游分支提交的消息里有一行 `Upstream-Commit: <上游完整提交号>`，记录它对应上游的哪个提交。

### 自动检查

`scripts/upstream-sync/check.py` 只读地回答「上游自上次同步以来有没有新提交」：拉上游 `main`，列出新提交（带 ⚠ 的可能与安全有关，先读具体改动），点出同步前要重点审的文件（依赖、工作流、脚本、许可证）。起点取自 `upstream-sync` 分支顶端的 `Upstream-Commit:`。退出码：0 没有新提交，10 有新提交，2 检查本身失败（失败不会被当成「没有新提交」）。本机运行 `python3 -I scripts/upstream-sync/check.py`；它的测试不联网：`python3 -I scripts/upstream-sync/test_check.py`。动手改 relay-forge 之前先本机跑一次也很便宜。

GitHub 上的 `.github/workflows/upstream-check.yml` 每周一北京时间 09:17 左右跑同一个脚本：有新提交就开一个标题以 `[upstream-check]` 开头的 issue 写明来了什么（之后几轮只更新正文，不重复通知）；同步完成后下一轮自动关掉它；检查本身失败时这次运行会变红。手动跑：`gh workflow run upstream-check.yml`（测试用参数 `-f since=<上游提交号> -f dry_run=true`）。通知走的是 GitHub 的 issue 和失败运行提醒，**要在 GitHub 的通知设置里确认邮件或手机推送开着**。

### 步骤

取上游、审、改写、整树核对，是 2026-11-20 第一次同步里实际跑过的。「在临时工作树里提交」和「`git merge upstream-sync`」用一次演练验证过：在临时工作树里模拟了一个上游新提交，`git merge` 自动找到合并基，无冲突，只带进改动的文件。第一次同步本身因为基线要从零建，是用 `git commit-tree` 手工指定合并基的。

1. 拉取：`env -u NO_PROXY -u no_proxy git -c http.version=HTTP/1.1 -c http.proxy=http://127.0.0.1:7891 fetch --no-tags upstream main`
2. **先审再合**，这是第三方代码：

   ```
   BASE=$(git log -1 --format=%B upstream-sync | sed -n 's/^Upstream-Commit: //p')
   git log --format='%h %an | %s' $BASE..upstream/main     # 来了什么
   git diff --stat $BASE upstream/main                      # 动了哪些文件
   ```

   通读 diff，重点看：`go.mod`、`go.sum`、`package.json`、锁文件（新依赖）；脚本和工作流；新增的外联地址；`LICENSE`、`NOTICE`；有没有安全相关的修复。
3. 在临时工作树里，把上游的改动按规则放到 `upstream-sync` 的顶端：

   ```
   git worktree add --detach /tmp/sync-wt upstream-sync
   python3 -I scripts/upstream-sync/rewrite.py delta "$PWD" /tmp/sync-wt $BASE upstream/main > /tmp/sync-go.txt
   (cd /tmp/sync-wt && xargs gofmt -w < /tmp/sync-go.txt && git add -A && git commit -F <消息文件>)
   ```

   消息仿照 `git log upstream-sync` 里已有的写法，末尾有 `Upstream-Commit:` 行。
4. 整树核对，逐文件证明「上游分支 = 上游最新树按规则改写」：`python3 -I scripts/upstream-sync/verify_tree.py "$PWD" <新提交> upstream/main`，必须是 `mismatches: 0`。通过后 `git branch -f upstream-sync <新提交>`，再 `git worktree remove /tmp/sync-wt`。
5. 合并：`git merge upstream-sync`。有冲突就只会出在自己改过的文件里，两边的改动都要保留。
6. 测试，三套都要跑（根目录的 `go test ./...` 不含嵌套模块）：

   ```
   go build ./... && go vet ./... && go test -count=1 ./...
   for m in relaykit tokenkit; do (cd $m && GOWORK=off go build ./... && go vet ./... && go test -count=1 ./...); done
   (cd web && bun run typecheck && bun run test)
   ```

### 同步记录

- 2026-11-20：从 `48f9df9` 同步到 `8542c9d`，3 个提交，无冲突：模型广场不再向访客显示私有分组（定价列表、分组倍率、性能指标都按访客可用分组过滤，管理员看板改走独立的管理员接口 `GET /api/data/perf-metrics/summary`）；Gemini 4 的思考档位按系列前缀匹配；Kling 增加 4:3 和 3:4 画幅。
