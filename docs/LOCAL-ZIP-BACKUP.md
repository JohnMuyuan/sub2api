# 本地 ZIP 数据库备份

系统设置 → 数据备份 → **本地 ZIP 备份**，保存绝对目录后，在“备份操作”的存储位置选择本地 ZIP，然后创建备份。定时备份配置也有相同的存储位置选项，复用现有 cron、保留天数、保留份数和月度归档规则。

备份不依赖 S3。ZIP 内的 `database.sql` 为既有 PostgreSQL 全库导出，包含数据库中的账号、配置、用量等数据。备份记录支持下载、删除和通过原有管理员密码确认流程恢复。SQL 内可能包含账号凭据等敏感数据，应妥善保管。

环境变量、TOTP/凭据加密密钥、外部图片对象、服务器配置文件不属于数据库导出，请单独保存。尤其需要保留原部署的固定加密密钥，否则数据库恢复后，已加密的字段可能无法解密。

## Docker / 1Panel

填写的是**容器内部路径**。如果现有部署已持久挂载 `/app/data`，可以选 `/app/data/backups`，无需额外挂载。

需要存到宿主机指定位置时，在原应用容器增加挂载，例如：

```yaml
volumes:
  # 保留原有挂载，再增加这一项
  - /opt/sub2api-backups:/app/backups
```

然后在页面填写 `/app/backups`。宿主机 `/opt/sub2api-backups` 需要给予容器运行用户写权限；当前自定义镜像使用 UID/GID `1000:1000`。请仅调整新建备份目录的权限。

目录不应放在网站公开目录中。ZIP 文件以仅运行用户可读写的权限创建；新目录默认仅运行用户可访问。备份生成成功前使用临时文件，失败时清理临时文件。大文件在浏览器下载时会占用浏览器内存，也可以直接从服务器备份目录取走 ZIP。

修改保存目录只影响以后创建的备份，旧记录继续使用原目录。多实例部署应使用各实例可访问的共享挂载；本地备份保留在生成它的磁盘上。定时保留策略针对备份记录统一执行，包括 S3 和本地 ZIP。

## 校验

```sh
cd backend
go test -tags unit ./internal/service -run 'Test(LocalZIP|Backup|Restore)' -count=1
go test ./internal/handler/admin ./internal/server/routes -run TestBackup -count=1
golangci-lint run --timeout=10m
```

```sh
cd frontend
pnpm exec vitest run src/views/admin/__tests__/BackupView.spec.ts
pnpm run build
```

本地路径访问由 `os.Root` 约束文件边界；记录中的文件名和 ID 也会校验。下载与目录修改接入原有管理员 step-up 验证；恢复继续要求管理员密码。
