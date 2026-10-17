# 自定义版本维护

## 仓库和目录

- 自己的仓库：https://github.com/JohnMuyuan/sub2api
- 跟随的上游：https://github.com/ranxi2001/sub2api，分支 `production`。
- 日常开发分支：`custom`。每个小功能单独提交。
- 本地工作目录：`D:\CodePorject\Exmaple\sub2api-custom`。
- 原解压目录 `sub2api-production` 保留，不参与更新。

`origin` 指向自己的仓库；`upstream` 指向 ranxi2001/sub2api。
这个上游已经按需同步 Wei-Shaw/sub2api，本仓库继续跟随它。

## 上游更新

GitHub Actions 的 **Custom upstream sync** 每天检查一次，也可以点 Run workflow 手动执行。
有更新时，在专用的 `sync/upstream` 分支尝试合并，并运行 Docker 构建验证。
成功后创建或复用合并到 `custom` 的 PR；原有自定义分支不会被自动合并。
有冲突或构建失败时，检查 Actions 日志，由维护者处理。`sync/upstream` 是机器人专用分支，请勿在上面开发。
如果已经存在更新 PR，后续同步会刷新其分支，合并前应查看最新同步运行的结果。

构建检查包括 Dockerfile 中的前端检查、前端打包和 Go 编译，不等同于完整测试。
GitHub 的 GITHUB_TOKEN 创建或更新的 PR 不会自动触发原有 PR CI；需要完整测试时让维护者额外运行。
GitHub 可能在公开仓库长期无活动后停用定时工作流，可在 Actions 中重新启用。

手动更新：

```sh
git switch custom
git pull --ff-only origin custom
git fetch upstream
git merge upstream/production
# 处理冲突并完成检查后
git push origin custom
```

## 自己的 Docker 镜像

Actions → **Custom build** → Run workflow（选择 `custom`）。
不勾选 publish 时只验证构建；勾选后发布：

```text
ghcr.io/johnmuyuan/sub2api:custom
ghcr.io/johnmuyuan/sub2api:sha-完整提交ID
```

当前工作流构建 `linux/amd64`，适用于 x86_64 服务器；ARM 服务器需要另行调整。
首次发布的 GHCR 包可能为私有；可在包设置中公开，或在服务器登录 GHCR 后拉取。
不要把访问令牌或生产 `.env`、数据库备份提交到仓库。

部署前备份数据库，并记录旧镜像的提交标签。将实际部署使用的 Compose 文件中应用镜像换为自己的镜像，再拉取并重建应用容器。
具体服务名和命令取决于现有部署；本次没有连接或修改服务器。
部署后不要使用应用内指向原作者发行版的一键更新来覆盖自定义版本。
代码、数据库迁移和部署变化需要一起检查；旧镜像不保证兼容新版数据库。

原有 Release 工作流仍保留。自定义镜像使用上述 Custom build，无需配置上游的其他发布凭据。
