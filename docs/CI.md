# 测试、构建与 GitHub 同步

CNB 是源码权威仓库，GitHub 为整仓镜像。顺序固定为：格式检查 → 模块校验 → vet → race 测试与覆盖率 → 构建 → 四平台交叉构建 → GitHub 镜像。任一步失败都终止。PR 和功能分支只验证；受信任的 main/tag 完成检查后同步所有分支与标签。

## 镜像范围

源：`https://cnb.cool/wuyin.keji/Tools/tempo-ai-answer`。
目标：`https://github.com/Maimai06/tempo-ai-answer`。

用户确认无需备份 GitHub 原历史。仅镜像 `refs/heads/*` 与 `refs/tags/*`，包括删除 CNB 已不存在的分支/标签。Issue、PR、Release、Actions 运行记录和仓库设置不属于 Git 对象，不做迁移。不要在 GitHub 镜像上独立开发。分支保护阻止覆盖时，需调整镜像仓库规则。

脚本从 CNB 拉取完整历史到独立临时裸库，不使用 CI 浅克隆。记录目标每个 ref 的对象 ID，用逐 ref 的 `--force-with-lease` 和单次 `--atomic` 推送。创建、覆盖、删除必须全部成功；远端并发变更则失败，不自动放宽 lease。源事件对应提交变化时拒绝镜像，等待新事件重新验证。

## 配置凭据

1. 创建仅允许目标 GitHub 仓库的 fine-grained PAT，Contents: Read and write；因为包含工作流，还需要 Workflows: Read and write。
2. 放入 CNB 密钥仓库文件，`allow_slugs` 仅允许 `wuyin.keji/Tools/tempo-ai-answer`。文件包含 `GITHUB_TOKEN`，不要把真实内容提交到源码仓库。
3. 已按用户提供的地址配置镜像阶段 imports：`https://cnb.cool/wuyin.keji/MM/api-token-key/-/blob/main/env.github.yml`，变量名 `GITHUB_TOKEN`。需在密钥仓库允许当前源仓库访问；未授权或 Token 无写权限时流水线失败。
4. 本地已有 Git 凭据时可执行 `go run ./tools/mirror --dry-run`，核对后运行 `go run ./tools/mirror`。Token 经临时 askpass 读取，不嵌入 remote URL 或命令参数。

功能分支与 PR 不导入密钥；合并 main 前审核 CI 与镜像脚本。GitHub Actions 仅验证/上传构建产物，不反向推送，没有同步回环。独立删除分支后，下一次 main/tag 事件或可信环境手动运行镜像使其收敛。

## 构建与验收

`go run ./tools/build` 使用 `CGO_ENABLED=0`，生成 Windows x64、macOS x64/arm64、Linux x64 的轻量 ZIP、构建清单和 SHA256SUMS。记录 commit、Go 版本与模块摘要。当前产物标为 executable-only，需要本机浏览器，不等于 PRD 含浏览器完整包。

完整包必须锁定浏览器来源、版本、摘要，携带许可证，并在支持的干净系统做断网启动/CDP 测试，验证 Linux 浏览器依赖和 macOS 隔离属性；未满足时不能发布为完整可用 v2。交叉编译不代表签名、公证或运行验收。GitHub 原生矩阵验证命令行与逻辑；真实平台与 DeepSeek 登录需人工验收。

`bash scripts/check.sh` 要求 scope/prompts/codemerge/config 各包语句覆盖率 ≥90%。镜像回归测试仅操作临时本地裸库，覆盖不同历史、标签、删除、并发 lease 与幂等。

语法依据：[CNB grammar](https://docs.cnb.cool/zh/build/grammar.md)、[环境变量](https://docs.cnb.cool/zh/build/env.md)。

CNB 构建附件通过官方 `cnbcool/attachments` 插件保留在对应 commit 下 14 天，不自动创建 Release。插件使用临时 CNB 票据，不导入 GitHub 密钥。GitHub 原生矩阵开启 `TEMPO_BROWSER_TEST=1`，用 loopback 验证真实浏览器 Cookie、网关头、表单和异步 JS。
