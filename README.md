# Tempo AI 自动答题助手

Go v2 迁移版：通过本机 Edge / Chrome 驱动 Tempo 平台与 DeepSeek 网页，监控面板内嵌于可执行程序。当前分支仅维护 Go 实现；旧版 Python 源码、依赖文件和启动脚本已移除，历史版本保存在 [python-v1 tag](https://cnb.cool/wuyin.keji/Tools/tempo-ai-answer/-/tree/python-v1)。

当前为开发分支，尚未完成真实平台和全部系统的发行验收。流水线生成的是 **轻量包**，需要已有 Edge / Chrome；带 Chromium 与 Linux 必要库的完整免依赖包仍需完成资源锁定与干净系统验证，不能把轻量包视为完整包。

## 使用

解压与系统匹配的 ZIP。Windows 使用 `tempo-ai-answer.exe`，macOS / Linux 使用 `tempo-ai-answer`；macOS 另附 `启动.command`。Go 支持在一台机器上交叉编译不同目标，但生成的二进制仍区分系统和 CPU 架构：Windows x64、macOS Intel、macOS Apple Silicon、Linux x64 各用对应的包。

首次运行生成占位配置，填写平台地址、账号和密码后再次启动：

```text
tempo-ai-answer login
tempo-ai-answer list
tempo-ai-answer run --scope "第1章 1-5"
tempo-ai-answer run --scope "1-3" --steps "1-3,8" --limit 3 --dry-run
tempo-ai-answer run --demo
tempo-ai-answer check --local
tempo-ai-answer check
tempo-ai-answer ai-test
```

无参数进入交互菜单。`--demo` 使用假 AI 并强制不提交；`--dry-run` 使用真实 AI 并禁止编程题与选择题提交。`--headless` 后台运行，不自动打开监控面板。首次登录请用可见模式完成平台验证码及 DeepSeek 登录。

配置、独立浏览器 profile 和日志默认存放于系统用户配置目录下的 `tempo-ai-answer`：

- Windows：`%AppData%/tempo-ai-answer`
- macOS：`~/Library/Application Support/tempo-ai-answer`
- Linux：`${XDG_CONFIG_HOME:-~/.config}/tempo-ai-answer`

可用 `--data-dir "路径"` 更改数据位置；`--config "旧版/config.json"` 直接读取 v1 配置。不要把个人配置、profile 或日志加入仓库。升级替换程序不会覆盖上述用户数据。

`check --local` 只检查本机资源、目录写权限及浏览器候选，不读取账号或访问远端；`check` 另检查平台及 AI。浏览器依次尝试 Edge、Chrome、随包 Chromium，不接管日常浏览器 profile。临时指定浏览器路径可用 `TEMPO_BROWSER`；程序启动时不下载浏览器。

## 开发与流水线

开发需要 Go **1.26.5**、Git 和用于 CDP fixture 的浏览器；用户运行已构建的程序无需 Go / Python / Node。

```sh
go mod download
bash scripts/check.sh
TEMPO_BROWSER_TEST=1 bash scripts/check.sh
go run ./tools/build
```

支持交叉构建 Windows amd64、macOS amd64/arm64、Linux amd64；ZIP 和 SHA256SUMS 输出到 `artifacts/release/`。macOS / Linux 文件若经过不保留执行权限的解压工具处理，需恢复执行权限；优先使用系统解压工具。macOS 签名与公证另行验收。

[流水线设计与密钥配置](docs/CI.md) · [迁移与验收记录](docs/MIGRATION.md) · [PRD](docs/PRD.md) · [开发规范](agent.md)

CNB 为主仓库。测试与构建通过后，源仓库的 push/tag 流水线把全部分支与标签同步至 [GitHub 镜像](https://github.com/Maimai06/tempo-ai-answer)。GitHub 的分支和标签以 CNB 为准，不在 GitHub 独立提交。同步不迁移 Issue、PR 或 Release。
