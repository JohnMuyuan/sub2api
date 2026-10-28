# 定向解除孤立模型隔离

实现与验收跟踪：本变更为管理员 ownership 预览和显式确认增加定向模型移除流程。确认必须在同一事务内验证预览、规则/账号版本与运行 lease，解除安全可识别的本规则独占标记并收缩规则范围。其他模型、分组与历史保留；普通编辑不能清除 server-owned 隔离。

验收状态：实现、PostgreSQL/HTTP 回归、旧源码 RED / 新源码 GREEN、前端 preview-confirm-readback 均 pending。证据将对应 PR 精确 head；本机禁止 Go 动态测试、Docker 与全量前端 build/lint/typecheck。
