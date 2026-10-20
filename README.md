# Tempo AI 自动答题助手

Go v2 迁移版：通过本机 Edge / Chrome 驱动 Tempo 平台与 DeepSeek 网页，监控面板内嵌于可执行程序。当前分支仅维护 Go 实现；旧版 Python 源码、依赖文件和启动脚本已移除，历史版本保存在 [python-v1 tag](https://cnb.cool/wuyin.keji/Tools/tempo-ai-answer/-/tree/python-v1)。

当前为开发分支，尚未完成真实平台和全部系统的发行验收。流水线生成的是 **轻量包**，需要已有 Edge / Chrome；带 Chromium 与 Linux 必要库的完整免依赖包仍需完成资源锁定与干净系统验证，不能把轻量包视为完整包。

## 使用

解压与系统匹配的 ZIP。Windows 使用 `tempo-ai-answer.exe`，macOS / Linux 使用 `tempo-ai-answer`；macOS 另附 `启动.command`。Go 支持在一台机器上交叉编译不同目标，但生成的二进制仍区分系统和 CPU 架构：Windows x64、macOS Intel、macOS Apple Silicon、Linux x64 各用对应的包。

首次运行在程序同目录生成 `config.json`，填写平台地址、账号和密码后再次启动。默认始终读取可执行文件所在目录的配置，从其他工作目录启动或双击 `启动.command` 也使用同一份文件；配置错误会显示实际路径。

```text
tempo-ai-answer login
tempo-ai-answer classrooms
tempo-ai-answer list
tempo-ai-answer --classroom "课堂ID" run --scope "第1章 1-3" --steps "1-2" --limit 2 --dry-run
tempo-ai-answer run --scope "第1章 1-5"
tempo-ai-answer run --scope "1-3" --steps "1-3,8" --limit 3 --dry-run
tempo-ai-answer run --demo
tempo-ai-answer check --local
tempo-ai-answer check
tempo-ai-answer ai-test
```

无参数进入交互菜单。选择“正式运行”“流程演示”或“单关试运行”后，先选择课堂，再按列表序号选择实验与关卡范围，例如实验 `1-3,5`、关卡 `1-2`。实验序号是本次列表的行号，列表同时显示章节编号；关卡范围分别应用于每个已选实验。输入 `0` 返回，留空会重新提示，不会默认选择全部实验或关卡。开始前显示最终实验清单和关卡范围；仅有一个课堂时自动选中。

菜单逐行说明每个功能，启动时显示版本与实际配置路径。首次测试建议选择 **7 单关试运行**：按提示选择课堂、实验和关卡，用真实 DeepSeek 最多处理一关，强制不提交答案或评测；该模式允许测试已完成关卡，只影响本次运行，不修改配置。**4 流程演示**使用模拟 AI，**3 正式运行**会提交答案与评测。确认页显示模式、课堂、实验、关卡和数量上限，回车开始、输入 `0` 返回主菜单。

交互范围输入支持中文逗号，如 `1-3，5`。错误序号、倒序范围和空输入分别提示如何修改。连接、登录、配置及平台数据错误会显示排查建议；完整技术错误写入提示位置的私有日志。控制台日志仅保留中文提示，日志文件继续记录时间、级别和脱敏后的诊断信息。试运行结果区分“生成答案”与正式运行的“评测通过”，未处理关卡时会提示检查范围与完成状态。

命令行可用 `classrooms` 查看课堂 ID，以 `--classroom "ID"` 指定课堂（优先于配置中的 `platform.classroom_id`），用 `--course "名称关键字"` 或 `--scope "第1章 1-3"` 选择实验，再用 `--steps "1-2,5"` 选择关卡、`--limit 1` 限制总处理数。`--scope "1-3"` 单独使用时表示第 1 章第 3 个实验。多课堂且没有指定课堂时需要交互选择；`--yes` / `--headless` 要求明确实验范围，多课堂时还需明确课堂，避免后台运行误选第一课堂或全部实验。命令行已显式指定实验范围但未指定 `--steps` 时，处理所选实验内未跳过的关卡。

`--demo` 使用假 AI 并强制不提交；`--dry-run` 使用真实 AI 并禁止编程题与选择题提交。`--headless` 后台运行，不自动打开监控面板。首次登录请用可见模式完成平台验证码及 DeepSeek 登录。默认跳过已完成关卡；交互选择仅作用于本次运行，不改写个人配置里的课堂 ID。

`config.json` 默认随程序放置，换机器时可随程序一起复制。独立浏览器 profile 和日志默认存放于系统用户配置目录下的 `tempo-ai-answer`：

- Windows：`%AppData%/tempo-ai-answer`
- macOS：`~/Library/Application Support/tempo-ai-answer`
- Linux：`${XDG_CONFIG_HOME:-~/.config}/tempo-ai-answer`

可用 `--data-dir "路径"` 更改 profile 和日志位置；显式指定该参数且未传 `--config` 时，也从该数据目录读取 `config.json`。配置路径优先级为 `--config` → 显式 `--data-dir` 中的 `config.json` → 程序同目录的 `config.json`。显式相对路径以终端当前工作目录为基准；`--config "旧版/config.json"` 可直接读取 v1 配置。旧测试包曾使用系统目录配置，新版默认使用程序同目录，请将已填写的文件放到程序旁边。不要把个人配置、profile 或日志加入仓库或分享给他人；升级替换程序时保留已有配置和用户数据。

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
