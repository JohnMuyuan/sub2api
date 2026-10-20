# Claude 开发交接

这是一份接手索引及已核实的项目快照。核对日期：2026-10-19（America/Los_Angeles）；代码基线：`bc1ab6f5bf27ccf62f355c6eeeeca3455e69b120`。继续开发前先查看实际分支、工作区和 GitHub 状态，不能把本文的快照当成永久现状。

## 1. 仓库、目录与协作方式

- 用户的个人仓库：<https://github.com/JohnMuyuan/sub2api>；主要分支为 `custom`。
- 上游仓库：<https://github.com/ranxi2001/sub2api>。用户希望保留自己的功能，同时跟随上游正式 Release。
- 当前 Windows 开发 checkout：`D:\CodePorject\Exmaple\sub2api-custom`。它有 Git 历史和远端，是之前提交 PR 的目录。
- `D:\CodePorject\Exmaple\sub2api-production` 是最初下载解压的源码目录。不要把两个目录当成同一个 Git checkout，也不要覆盖原解压目录来代替 Git 合并。
- 新功能从最新 `origin/custom` 创建独立分支，向 **JohnMuyuan/sub2api 的 custom 分支**提交 PR。之前由用户检查并合并 PR；用户没有要求向原上游提交改动。
- 先检查 `git status` 和已有改动，保留用户或其他开发助手的工作。不要重写已发布历史、为了同步上游清掉自定义功能，或把生产部署当作代码提交的一部分擅自执行。
- 本机开发助手的文件写入权限不等于 GitHub 权限。以前对 `sub2api-custom` 的写入需要本机沙箱批准；这不是向上游作者申请权限。新会话应按其实际权限处理。

推荐开始时运行：

```sh
git status --short
git remote -v
git fetch origin
git log -5 origin/custom --oneline
```

## 2. 自动同步：只跟正式 Release

用户明确要求：**合并正式 Release 对应的代码，不追踪上游分支中的未发布提交。**

- 入口：[custom-upstream-sync.yml](.github/workflows/custom-upstream-sync.yml)。每小时第 7、37 分钟检查；GitHub 排队可能延迟。也有 custom push 和手动触发入口。
- 目标解析：[custom_upstream_release.py](.github/release-tools/custom_upstream_release.py)。通过 GitHub `releases/latest` 查询正式版本，拒绝 draft/prerelease，再获取该 Release 的 `tag_name` 对应 Git 标签。
- 必须合并标签剥离后的实际 commit。不要改回 `upstream/production`，也不要用 Release 的 `target_commitish` 替代标签；它可能指向持续变化的分支。
- 已包含该发布提交时直接结束，不重复合并或发布。网络/API/标签解析失败时停止，不能回退成跟踪分支。
- 新候选先进入 `sync/upstream`，对同一个候选 SHA 执行自定义构建、CI、安全扫描和涉及相关文件时的 re-login runtime 检查；全部符合条件才推进 `custom` 并派发发布。
- 推进 `custom` 不使用强推；若用户同时提交了新改动，自动同步应停止。`sync/upstream` 临时候选分支使用显式 lease 更新。
- 仍可能有真实代码冲突；检查通过不代表生产环境已完成部署。此前已合入的未发布上游提交没有被回滚。

已合并修复：

- [PR #13](https://github.com/JohnMuyuan/sub2api/pull/13)：修复测试清单冲突与机器人 PR 审批过期的问题；自定义前端关键测试移到 `Makefile.custom`，保留上游原清单。现在不再创建立即合入的上游机器人 PR，改用可复用工作流先检查。
- [PR #14](https://github.com/JohnMuyuan/sub2api/pull/14)：切换到正式 Release 标签同步，2026-10-19 已合并。
- 对应测试：`test_custom_sync.py`、`test_custom_upstream_release.py`。它们含真实 Git 合并、轻量/附注标签，以及“分支已前进但只选择发布提交”的回归场景。

## 3. 自用发布、Docker 和软件内更新

- 发布入口：[custom-build.yml](.github/workflows/custom-build.yml)。`custom` 分支 push 会发布自用构建；手动发布也支持。因此，**我们自己修改代码或文档也可能生成新自用版本，并不需要上游再发 Release**。
- 自用镜像：`ghcr.io/johnmuyuan/sub2api:custom`。还发布具体版本与 `sha-<commit>` 标签。不要指导用户继续拉原上游镜像来获取自定义功能，也不要假设存在自用 `latest` 标签。
- 当前自用构建为 `linux/amd64`；用户服务器已确认是 x86_64/amd64。修改架构支持需核对主程序、资源与托管登录 runtime 是否匹配。
- 版本格式由构建脚本生成：UTC 年、月，加 `GITHUB_RUN_NUMBER`，即 `YYYY.M.<构建序号>`。末段不是日号；同一天可有多个不同版本，不应仅按日期生成重复版本号。
- `UPDATE_REPO` 构建参数将后端更新器、前端及托管登录 runtime 指向自用仓库。Dockerfile 默认值仍可能是上游，手动构建时不能漏掉参数。
- 发布会生成与镜像匹配的更新归档、runtime 和校验文件；先创建草稿，上传完整资源并推送镜像后才正式发布。过期排队构建会检查 `custom` HEAD，避免覆盖更新版本。
- 发布说明由 [custom_release_notes.py](.github/release-tools/custom_release_notes.py) 生成，附已包含的上游正式版本说明。自用版本号与上游基线版本号是两个概念。
- 更新器代码：[update_service.go](backend/internal/service/update_service.go)；容器启动逻辑：[docker-entrypoint.sh](deploy/docker-entrypoint.sh)。自用镜像支持把更新后的可执行文件保存在 `/app/data/runtime`，相同镜像重启保留内置更新；换新镜像会按镜像哈希初始化其程序。
- 用户用 1Panel Docker 部署，已有宿主机目录挂载到 `/app/data`。保证数据持久化；备份目录可填容器内 `/app/data/backups`。不要为修复更新删除该挂载目录。
- “an update or rollback is already installed”等待重启错误应先核对是否确已安装、需要重启，不要重复覆盖或删除程序来绕过状态。
- `.gitattributes` 为 README 配置了 `keep-custom-readme` 合并驱动；同步命令通过 `git -c merge.keep-custom-readme.driver=true ...` 启用。当前 README 已被后续文档提交调整，应保留实际最新内容，不根据早期聊天重置。

核对时自用最新发布为 `v2026.10.20`，构建来源为上述基线；Custom build 运行 `39240571742` 成功。版本名中的 20 是构建序号，不是这份文档的日期。上游正式基线为 `v2.12.2`。以后请查询实时状态。

## 4. 已有自定义功能及入口

这里列出用户关心的改动及当前实现入口，不是所有代码的作者归属清单。上游和其他助手也持续修改过这些功能；以当前代码和测试为准。

| 功能 | 后端入口 | 前端/其他入口及要点 |
| --- | --- | --- |
| 账号请求时区、按实际出口自动选择 | `backend/internal/service/account_request_timezone.go`、`openai_request_timezone_egress.go`、`backend/internal/repository/http_upstream_account_timezone.go` | `frontend/src/components/account/OpenAIRequestTimezoneField.vue`、`frontend/src/components/settings/RequestTimezoneSettings.vue`；账号 `extra.request_timezone` 支持留空、手动时区和 `auto`。只对支持的请求信息改写，按实际代理出口解析；不要把它说成修改出口 IP 或服务器时区。 |
| 全站统计时区 | `backend/internal/pkg/timezone/`、`backend/internal/service/setting_system_timezone.go`、`backend/internal/repository/system_timezone.go`、`custom_group_usage_rollup_repo.go` | `frontend/src/components/settings/SystemTimezoneSettings.vue`；涉及日期展示、统计与额度边界。设置保存后在启动时应用，需要重启；不能在一个 HTTP 请求里随意修改全局时间和缓存。账号显式额度时区等覆盖规则见测试。台湾时区为 `Asia/Taipei`。 |
| 本地 ZIP 备份、手动及定时、导入恢复 | `backup_local.go`、`backup_import.go`、`backup_restore_state.go`、`backup_retention.go`、`backup_service.go`（均在 `backend/internal/service/`） | `frontend/src/views/admin/BackupView.vue`、`frontend/src/components/admin/BackupArchiveSettings.vue`、`frontend/src/api/admin/backup.ts`。当前页面和归档功能有后续演进，不要只按最早“系统设置”页面描述开发。 |
| 账号只读权限组与邮箱脱敏 | `backend/internal/service/account_viewer.go`、`observer_scope.go`；`backend/internal/server/middleware/observer_routes.go`；`backend/internal/handler/admin/observer_account_access.go` | 独立 `account_viewer` 角色基于授权组范围读取数据，与普通 observer/admin 区分。后续已调整为复用观察者相关入口；旧 `AccountPoolReadOnlyView` 页面已删除。必须服务端鉴权和脱敏，覆盖账号/组标签等可能含完整邮箱的字段，不能只靠前端隐藏。 |
| Claude 请求一致性诊断与重试保护 | `backend/internal/service/gateway_claude_request_safety.go`、`gateway_service.go` | 相关测试为 `gateway_claude_request_safety_test.go`。诊断版本、会话、流式等一致性；限制重发，处理 401/403、Retry-After、已经输出响应和发送状态不确定的情况。保护状态在单次请求内共享，不记录提示词或会话凭据；修改时保留脱敏和避免重复生成的语义。不能承诺规避识别或封号。 |
| 随机自动停调阈值（账号级与平台级范围） | `backend/internal/service/account_scheduling_threshold_random.go`、`setting_account_scheduling_threshold_ranges.go`；接入 `account_scheduling_threshold_eval.go` 与 `RateLimitService.ApplyAccountSchedulingThreshold` | `frontend/src/components/settings/AccountSchedulingThresholdRangeSettings.vue`、`SchedulingThresholdRangeFields.vue`、`components/account/SchedulingThresholdRounds.vue`。每个窗口按「账号 ID + 窗口 + 重置时间（取整到小时）」哈希在范围内取值，同窗口稳定、重置后换值；优先级为账号范围 > 账号固定值 > 平台范围 > 平台固定值。抽到 100 表示本窗口不主动停调；可设每种窗口每个自然月（全站时区）抽到 100 的次数上限，记录在 `extra.scheduling_threshold_full_draws_<窗口>`。账号范围存于 `credentials.account_scheduling_threshold_range`，同时把下限写入原 `account_scheduling_threshold` 以兼容旧版本。每个窗口还可单独设固定阈值或随机范围（`account_scheduling_threshold_window_rules.go`；Go 文件名不能以 `_windows.go` 等系统名结尾，否则只在该系统编译）：账号存于 `credentials.account_scheduling_threshold_windows`，平台存于设置 `account_scheduling_threshold_window_rules`；查找顺序为账号该窗口 > 账号默认 > 平台该窗口 > 平台默认，Fable（`7d_oi`）只停 Fable 模型。新键需保留在调度快照投影中。 |
| 自动开启空闲额度窗口（OpenAI OAuth、Anthropic OAuth/Setup Token） | `backend/internal/service/account_window_prime.go`、`account_window_prime_runner.go`、`setting_account_window_prime.go`；挂在 `ScheduledTestRunnerService` 的启停上，每分钟检查 | `frontend/src/components/settings/AccountWindowPrimeSettings.vue`、`components/account/AccountWindowPrimeFields.vue`、`AccountWindowPrimeStatus.vue`。窗口重置后一直未被使用时，在设定延迟（默认 1–5 分钟，按账号与重置时间哈希）后经 `AccountTestService` 发一条短请求；平台默认关闭，账号可设为跟随/开启/关闭，模型和提示词（多条随机挑选）可配置，模型下拉框在系统设置里列内置模型、在账号里列该账号的上游模型（`getAvailableModels`）；可选按全站时区的时段限制。Redis 锁保证多实例只发一次，每次重置最多尝试 3 次，结果写在 `extra.window_prime_state`。Claude 测试请求原本不记录窗口头，这里通过上下文回调交给 `UpdateSessionWindow`。会消耗少量额度，不能承诺上游识别不出自动请求。 |

### 备份恢复的版本差异

- 新本地 ZIP 包含 `database.sql` 和 `manifest.json`，清单校验 SQL SHA-256，并记录 `keys_in_database`。不能沿用早期“ZIP 只有 SQL”的说明。
- 密钥持久化/启动兼容见 `backend/internal/repository/security_secret_bootstrap.go`；恢复流程见上述服务和 `backend/internal/repository/backup_pg_dumper.go`。有新旧归档兼容逻辑，不要直接假定任意历史 ZIP 都能完整迁移到新服务器。
- 导入会先验证归档，再登记备份；破坏性恢复是后续独立动作。保留操作互斥、校验、状态持久化及 SQL 失败即停止等保护。
- 当前上传上限 256 MiB、解压 SQL 上限 8 GiB，来自 `backup_import.go`，以后可能变化。更改时同步前后端和代理限制。
- ZIP 不是整台服务器快照；容器环境变量、部署配置、外部文件/对象与存储需要另行核对。数据库与归档含敏感数据，不提交到仓库。

### 以前的登录 429 排查线索

用户使用 1Panel 网站反代，外层还有 Cloudflare/CDN；历史日志曾把请求 IP 记为 Docker 网关 `172.18.0.1`。这只是排查线索，不是已经确认的唯一根因，也不代表当前仍有故障。

若复发，结合登录时间读取应用/反代日志，核对真实客户端 IP、可信代理链、Redis 健康和限流键。入口包括 `backend/internal/server/routes/auth.go`、`backend/internal/middleware/rate_limiter.go`、`auth_abuse_guard.go`。不要直接关闭登录保护、清空 Redis，或无条件信任任何来源伪造的转发头。

## 5. 开发和验证

后端模块根是 `backend/`，Go 版本以 `backend/go.mod` 为准（快照为 1.27.2）。前端 Vue 3 / TypeScript，使用 pnpm 9；CI 使用 Node 20、Python 3.12。不要在仓库根运行 Go 模块命令。

发布/同步修改优先跑：

```sh
python -m unittest discover -s .github/release-tools -p 'test_custom_*.py'
git diff --check
# 在 backend 目录运行；环境需能获取 actionlint：
go run github.com/rhysd/actionlint/cmd/actionlint@latest -shellcheck= -pyflakes= ../.github/workflows/custom-upstream-sync.yml
```

前端在 `frontend/` 运行：

```sh
corepack pnpm@9 install --frozen-lockfile
corepack pnpm@9 run typecheck
corepack pnpm@9 run lint:check
corepack pnpm@9 exec vitest run <受影响的测试路径>
```

有 GNU make 的环境可在根目录运行 `make test-frontend`：包含 lint、类型检查和关键回归。个人关键测试追加在 `Makefile.custom`，不要重新插回上游清单顶部。Windows 本机会话没有可直接调用的 make，使用对应命令或 Linux CI；不要因为缺少 make 就报告所有检查已通过。

自定义后端检查的精确命令见 `.github/workflows/custom-build.yml` 的 `Verify custom feature regressions`。完整 CI 入口为 `backend-ci.yml`；安全检查为 `security-scan.yml`；相关托管登录 runtime 为 `reauth-runtime.yml`。完整集成测试需要 Docker/PostgreSQL 等环境。

近期验证记录：PR #13 开发时本地前端关键回归 93 文件、1,496 测试通过；PR #14 开发时 14 项自用发布/同步测试与工作流语法检查通过。它们是历史结果，新修改后应运行适用检查，不能引用旧结果声称新代码通过。

## 6. 接手时的工作清单

1. 阅读本文，确认工作目录、分支、用户的新需求及 GitHub 上的实际状态。
2. 在最新 `origin/custom` 上开新分支，保留时区、ZIP 恢复、只读权限与脱敏、重试保护、自用发布能力。
3. 依改动范围运行检查。若失败，读具体 job 日志，区分编译/测试错误、合并冲突、审批过期、网络失败；不要把邮件里的 failure 一概解释为程序坏了。
4. 向个人仓库创建 PR，说明行为变化、验证结果与待完成事项。当前流程中普通 custom push 也会发布，文档改动同样可能触发构建。
5. 更新本文中受影响的事实。保持 README 和已有英文文档的最新内容，不在文档、日志或提交中添加账号密码、Token、Cookie、真实业务备份或服务器密钥。

可以给 Claude 的启动语：**“先读 HANDOFF.md，核对最新 origin/custom 与工作区，保留现有自定义功能；今后的上游同步只能针对正式 Release。然后完成我接下来提出的需求，并按改动范围验证和提交 PR。”**
