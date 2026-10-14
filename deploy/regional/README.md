# 跨区域运行配置与验收

本目录负责离线生成配置与部署验收。它不创建集群、不生成生产凭据、不发送模型请求，也不改变默认关闭的区域模型入口。配置生成通过不等于业务链路通过。

## 拓扑与前置条件

示例包含两个主站进程、两个区域，每区域两个 Pod。主站共用主站 PostgreSQL/Redis；同一区域的全部 Pod 共用该区域的 PostgreSQL/Redis；各区域数据库相互独立。共享账号 101 同时出现在两个区域，仅代表允许使用，不意味着两个区域分别拥有账号完整并发或 RPM。

当前每个进程只能发布一个区域的 auth/scheduler stream，控制 listener 也只接受一个 Scope。示例将 primary-a 分配给 region-a，primary-b 分配给 region-b；两者仍属于同一主站 SQL 预算与完整 epoch 成员表。不能把按区域发证的控制端口放到任意主站 Pod 轮询的 Service 后面。

在隔离开发机完成构建，固定构建 SHA 和镜像 digest；不要在生产服务器执行高内存编译。应用数据库需完成对应 PR 的增量迁移，区域库也需应用自身 schema。区域请求是否仍存在主站回源须由最终代码和请求测试核实，不能仅凭使用本地数据库配置断言。

## 离线配置生成

复制 topology.example.json 到工作区外的私有目录，更换实例名、区域、资源 ID、端点和证书指纹。example.invalid 与重复字母指纹是虚构样例。完整列举所有主站实例；遗漏旧 Pod、Job、旧版本守护进程会导致预算之外的请求继续运行。

单独准备权限 0600 的 secrets.json，结构如下，两个键均需由密钥系统生成的 32 字节随机值进行 base64 编码。每区域使用独立加密材料，不将此文件或生成结果加入 Git。

    {"regions":{"region-a":{"key_id":"key-v1","encryption_key":"BASE64_32_BYTES","auth_lookup_key":"BASE64_32_BYTES"},"region-b":{"key_id":"key-v1","encryption_key":"BASE64_32_BYTES","auth_lookup_key":"BASE64_32_BYTES"}}}

从仓库根执行：

    python3 deploy/regional/render.py --topology /private/topology.json --secrets /private/secrets.json --output /private/rendered
    python3 -m unittest discover -s deploy/regional -p 'test_*.py' -v

目标目录必须不存在；已有目录拒绝覆盖，以免误替换运行身份。目录权限 0700，JSON 权限 0600。每个实例有 runtime.json（合并进该实例实际应用配置，不代替数据库等完整配置）、auth.json、scheduler.json、concurrency.json。三个 runtime 路径分别也可通过 RUNTIME_REGIONAL_AUTH_FILE、RUNTIME_REGIONAL_SCHEDULER_FILE、RUNTIME_REGIONAL_CONCURRENCY_FILE 指定。

生成结果中的 MaximumLifetime 为 Go time.Duration 的纳秒整数；MaximumGrant 为 10^-9 USD 的十进制字符串，当前固定 1 USD 仅满足 slots-only handler 的 Scope 校验，不启用资金授权或代表真实金额限额。资金、RPM 与状态协调配置必须使用后续对应 runtime 的配置，不能把此数值当成所有维度的限制。

## 挂载与端口

将各实例自己的 JSON 只读挂载到 /run/sub2api-regional/。CA、证书、私钥分别挂载到 tls/ca.pem、tls/cert.pem、tls/key.pem；进程用户必须能够读取，私钥不得对其他租户开放。证书需包含实际内网服务名 SAN 与对应 serverAuth/clientAuth 用途。Scope 固定单一客户端证书 pin，因此当前同区域 broker 共用该区域身份；跨区域不可复用证书。证书轮换须逐阶段维护受信指纹，不临时取消校验。

| 端口 | 功能 | 允许访问者 |
| --- | --- | --- |
| 8080 | 既有 HTTP 网关、/health、/readyz | 内网入口；完成全部验收后才路由模型 |
| 9443 | mTLS scheduler snapshot 与 status | 指定区域副本、获授信监控 |
| 9444 | mTLS auth snapshot 与 status | 指定区域副本、获授信监控 |
| 9445 | mTLS 槽位 control | 对应 Scope 的区域 broker |

跨区防火墙仅允许指定对端访问控制端口，不发布到公网 ingress。区域快照持久路径为 /app/data/regional/<instance>/<kind>.json；创建父目录并确保进程可写。各 Pod 必须独占自己的 state 文件和 lock；区域 SQL 账本则必须共享。不要把两个 Pod 指向同一个快照文件，也不要给每个 Pod 使用各自独立账本，否则会重复使用租约。

bootstrap 默认为 false。新安装需在确认为全新 stream、没有历史 revision 后显式完成一次 bootstrap；已有安装保留 publisher revision、subscriber 文件、锁与账本，禁止通过删文件或永久打开 bootstrap 解决启动失败。备份中含账号凭据，应加密并控制访问。

## 切换与放量顺序

1. 保持模型入口关闭；记录旧 Pod UID、工作负载、构建 SHA、旧 Redis 身份，以及全部主站/区域新实例。
2. 关闭旧入口并停止接纳，等待所有 HTTP/SSE/WS 请求终止；记录控制器已收敛、旧 Pod 终止且不会被旧 ReplicaSet/Job 重新创建的事实。撤销旧 Redis 身份，确认无旧连接、旧槽位排空。
3. 所有主站切到 SQL authority，使用同一 epoch、完整 PrimaryInstances；检查 heartbeat。Cutover 指向 Kubernetes API、专用主站命名空间及固定 StatefulSet UID。读取实际 UID 填入 topology.cutover.workloads，各 Pod 名必须等于 StatefulSet 的 ordinal 清单；模板和 Pod 都带 sub2api.io/regional-epoch 注解。API 会核对 revision、Pod 成员、Ready 和节点心跳并保存不可变证据。Redis TTL 清空或布尔声明不能证明旧在途请求已经结束；失联节点上被强制删除的进程仍需隔离节点和确认终止，禁止强删后直接授予预算。
4. 验证 auth/scheduler revision、新鲜度、三维槽位、余额与 Key/订阅/账号/平台额度、共享 RPM、OAuth 所有权、冷却传播。完成真实应用路由的本地多 Pod mock 验收与故障注入后，才可对经授权的生产小流量另行验收。
5. 放量必须由最终受控入口检查全套依赖；不可直接移除 router 的 503 中间件，也不可把 /health 返回 200 当成可计费请求就绪。

## 观测与故障边界

存活使用 /health；进程排空状态使用 /readyz（以最终集成版本实现为准）。快照状态只在对应 mTLS listener 的 /internal/regional/auth/status 与 /internal/regional/scheduler/status 读取。关注 ready、revision、expires_at、attempts、failures、last_success；请求需使用被该 listener 信任的证书，不能绕过 pin。认证快照内容、原始 Token 和模型正文不可写入监控。

账本运维查询见 observe.sql：只输出计数、状态和累计保留额度，不读取请求 payload、用户密钥或事件正文。outbox 未确认、started 未终结、未完成 broker 操作和过期但占用的槽位必须独立告警。区域被网络分区时依赖有效租约和快照期限拒绝新请求，未知已发送消费继续保留；恢复后重放原 ID，不新建 ID 绕过确认丢失。

状态新鲜只代表快照有效；槽位存在只代表容量被授权；资金可用只代表保留成功。三者都不单独证明实际模型路径、取消与结算正常。

主站只读 inventory 权限模板见 inventory-rbac.yaml。必须替换命名空间和允许的 node resourceNames；该账号不能修改 Pod、控制器、节点或 Secret。向主站显式挂载 Kubernetes CA 和短期 projected token 到 /run/sub2api-inventory/ca.crt 与 token；区域 Pod 不需要集群 API 凭据。工作负载创建/删除权限由另外的管理员持有，专用命名空间不运行 Deployment、DaemonSet、Job 或 CronJob。Pod 替换导致 inventory 摘要变化，应先完成排空与新 epoch 审核，不通过删除证据记录恢复。

rpm_units 为每账号、每区域分钟配额，生成 Scope.MaximumRPM 与 RPMTargets，独立于并发 slots。主站负责全局分配；多个区域声明的容量可能超过账号余量，运行时应拒绝超额申请。区域时钟证明有期限，分钟边界保护可能暂时拒绝新请求；不要放大本地区域配额以规避拒绝。

## 排空与回滚

先从入口停止新增，再等待 HTTP/SSE/WS 终止；保留无法确认结果的 started 请求。等待 usage outbox 与 broker 操作确认，核对本地与主站账本及累计消费，按原租约身份执行 drain/close。只有确认关闭和余量返还后才能退回旧 Redis acquisition；TTL 到期不能自动返还。

回滚保留数据库增量表、永久请求/事件 ID、取消 tombstone、快照 revision 和价格版本。回退二进制不撤销迁移、不证明已经租出的资金与槽位回收。确认丢失或旧进程状态不明时停止新增并保留证据，不能清空账本、改 epoch 后重新授予预算。

## 验收证据

记录固定 base/head SHA、镜像 digest、完整命令、退出码、测试名、是否跳过及 mock 边界。配置单测、SQL/库集成测试、真实应用 mock 端到端、CI、生产验证分别记录。当前本目录的离线生成器测试不包含真实模型调用、计费、跨区域网络性能或生产部署。
