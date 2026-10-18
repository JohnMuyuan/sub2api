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

GitHub Actions 的 **Custom upstream sync** 每小时检查一次，也可以点 Run workflow 手动执行。
有更新时，在专用的 `sync/upstream` 分支尝试合并，并运行功能回归测试和 Docker 构建验证。
成功后创建或复用 PR，并将 `custom` 快进到通过验证的提交，随后自动发布镜像和内置更新包。
有冲突或构建失败时，检查 Actions 日志，由维护者处理。`sync/upstream` 是机器人专用分支，请勿在上面开发。
如果验证期间 `custom` 有新提交，自动合并会拒绝覆盖它，等待下次同步重新验证。

构建检查包括自定义功能的 Go 回归测试、Docker 持久更新测试、前端检查、前端打包和 Go 编译，不等同于完整集成测试。
验证由同步工作流直接调用，不依赖机器人创建的 PR 是否触发额外 CI。
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

提交或合并到 `custom` 后自动发布。也可以 Actions → **Custom build** → Run workflow（选择 `custom`）。
手动运行时不勾选 publish 只验证构建；勾选后发布镜像和 GitHub Release：

```text
ghcr.io/johnmuyuan/sub2api:custom
ghcr.io/johnmuyuan/sub2api:sha-完整提交ID
```

当前工作流构建 `linux/amd64`，适用于 x86_64 服务器；ARM 服务器需要另行调整。
首次发布的 GHCR 包可能为私有；可在包设置中公开，或在服务器登录 GHCR 后拉取。
不要把访问令牌或生产 `.env`、数据库备份提交到仓库。

部署前备份数据库，并记录旧镜像的提交标签。将实际部署使用的 Compose 文件中应用镜像换为自己的镜像，再拉取并重建应用容器。
具体服务名和命令取决于现有部署；本次没有连接或修改服务器。
首次换成新的自定义镜像后，应用内一键更新和回退均指向自己的仓库。部署迁移、持久目录和版本规则见 [CUSTOM-AUTO-UPDATE.md](CUSTOM-AUTO-UPDATE.md)。
代码、数据库迁移和部署变化需要一起检查；旧镜像不保证兼容新版数据库。

原有 Release 工作流仍保留。自定义镜像使用上述 Custom build，无需配置上游的其他发布凭据。
