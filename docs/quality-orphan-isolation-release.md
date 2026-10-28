# 定向解除孤立模型隔离

普通账号编辑继续保留 server-owned 隔离；普通规则编辑继续使用严格的模型/组范围全等检查。本变更复用 ownership reconciliation 的管理员预览与 fence，新增 `POST /admin/account-quality-plans/:id/ownership/remove-model`。请求指定一个 `model`、预览 `fence`、`confirm: true` 与枚举原因 `reason_code`（`orphan_marker_recovery` 或 `retire_model`），通过现有管理员审计中间件记录。

确认在 plan → account → state 锁顺序中重建预览，验证规则版本、账号版本、完整原始 ownership/隔离/分组和模型映射。运行 lease 未结束时阻断；确认后的规则版本变化使旧 claim/outcome 失效，不取消或重放调用。事务仅删除指定模型对应的隔离标记和旧归属，同时收缩规则配置和 ownership 的模型范围。其他模型、分组、策略、计数、历史保留；隔离变化通过原 scheduler outbox 失效出站缓存。

没有原快照时，只接受字段严格为 `reason: quality_rule:<本规则ID>` 和合法 `isolated_at` 的标准独占标记；明确告知“仅移除此标记，不恢复旧值”。有匹配快照时也只移除标准标记；已有快照但内容变更、其他 owner、无 owner、畸形字段、额外限制、非空旧限制、共享映射或歧义映射均保留。当前项缺失可仅移除该目标及旧归属。保留至少一个受测模型，最后一个模型仍走原独立恢复/删除流程。

前端选择目标、查看效果或阻断原因、选择操作原因、勾选确认后提交；再次读取预览成功才同步移除草稿模型，其余草稿保留。readback 失败明确提示确认可能已提交、需重新预览核对。规则本身在确认事务中已收缩，下一轮不会重新隔离被移除模型，其他草稿设置可随后普通保存。

验收状态：首轮远端精确 head `91c119cac64112009c84113f09547c0cb51bdfb3` 的旧源码 PostgreSQL RED、新源码 repository/HTTP 与原保护 GREEN 已取得；首轮前端夹具失败已修正，修正后本机定向 128/128 通过。当前 head 的远端完整复验与截图仍 pending。未运行或失败项不会标作通过；最终结果见 PR 验收清单与 CI artifacts。本机未运行 Go 编译/动态测试、Docker、全量前端 build/lint/typecheck，无生产操作或付费请求。
