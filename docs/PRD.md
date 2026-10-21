# Tempo AI 自动答题助手 v2 · 产品需求文档（PRD）

| 项 | 内容 |
| --- | --- |
| 产品 | Tempo AI 自动答题助手（美林 Tempo 实训平台） |
| 版本 | v2.0 —— Go 跨平台重写版 |
| 日期 | 2026-10-19 |
| 状态 | 草案 |
| 上游 | v1（Python + Playwright，归档于 `python-v1` tag，当前分支仅保留 Go 实现与冻结的 JSON 对照数据） |

---

## 0. 开发约束与来源

- 开发约束来源：[default-code-dev](https://cnb.cool/wuyin.keji/Tools/default-code-dev)，本次同步提交为 `3a1b94486093e9044446157e09bf54a38a98b203`（2026-10-15）。后续更新须显式核对上游差异。
- 根目录 `agent.md` 保持该提交原文；`.cnb.yml` 保留云开发环境，并按本项目扩展测试、构建与镜像阶段（见 `docs/CI.md`）；`.gitignore` 在上游模板后追加本项目的配置、Python 缓存和打包产物规则；`AGENTS.md` 为开发约束入口。
- 遵循 `agent.md`：独立 `feat/*` / `fix/*` 分支开发、中文 Conventional Commits、中文公共接口与复杂逻辑注释、共享逻辑抽离、必要测试和提交前自查。保留用户已有变更，仅暂存本次任务文件。
- CNB 云端开发环境使用上游 `.cnb.yml` 的 `default-code-env` 镜像。各目标系统和架构的发行包分别验证，单个 Linux 开发环境构建成功不能代替 Windows、macOS、Linux 的启动验收。
- 发布使用语义化 annotated tag，正式 v2 首发为 `v2.0.0`；tag 描述使用 Markdown，推送前确认工作区干净且对应提交已推送。

---

## 1. 背景

v1 已完整可用（平台登录、抓题、编程/选择题作答、diff 反馈重试、实时监控面板、对话清理），代码复查后确认无结构性缺陷，但存在以下已识别债务，需在重写中一次性解决：

| 编号 | 问题 | 影响 |
| --- | --- | --- |
| A1 | 重试时 prompt 的代码框架被污染（`codes` 原地修改，`build_prompt` 重复读同一对象） | 重试质量下降；片段合并时新旧代码叠加 |
| A2 | `before_chats` 空集合被当作「无前置对话」，首次运行不清理新对话 | 账号残留垃圾对话 |
| A3 | 监控面板「打开实验页」链接的 classroomId 取自 `cfg` 而非 `plat`，写盘失败时为空 | 链接失效 |
| A4 | 配置缺字段直接 KeyError；`base_url` 占位符未校验 | 旧配置升级即崩、误导性报错 |
| A5 | 浏览器 channel 硬编码 `msedge`，无 Chrome 回退 | 仅装 Chrome 的用户无法启动 |
| A6 | 非法范围输入裸 traceback；中文数字 >10 解析失败 | 输入体验差 |
| B1–B5 | 全局配置变量、完成度逻辑双份拷贝、跨层访问私有方法、`solver.py` 过重、注释与代码漂移 | 维护成本 |
| D1–D5 | 零测试、无 `.gitignore`、日志不落盘、死依赖 `requests`、README 与实现不符 | 回归风险与安全隐患 |

## 2. 目标与非目标

### 2.1 目标

- **G1 技术栈迁移**：Python → Go + chromedp，功能与 v1 对齐；生产程序保持纯 Go 依赖并使用 `CGO_ENABLED=0` 构建，使 Linux 构建环境可以生成所有首版目标二进制。
- **G2 债务清零**：A / B / C / D 全部问题在新设计中消除（见 §11.1 映射表）。
- **G3 可测试**：`scope` / `prompts` / `codemerge` / `config` 语句覆盖率分别 ≥90%，关键业务分支逐项列出并用表驱动用例覆盖；迁移期用 fixture 做 v1↔v2 行为对照。Go 标准覆盖工具报告语句覆盖率，不将它称为分支覆盖率。
- **G4 行为一致性**：同一题目输入下，v2 与 v1 的 prompt 生成、代码提取、多文件合并、答案解析输出一致；已确认缺陷的修复按 §11.4 单独记录预期差异。
- **G5 跨平台开箱运行**：支持 Windows、macOS、Linux，具体版本和架构见 §6.5。Go 主程序携带运行所需的 Go 运行时和内嵌网页资源，各目标完整包另附已验证的 Chromium 及必要资源；换机下载对应包、解压后即可启动，无需安装开发环境或依赖。主程序可单文件分发，完整包负责覆盖无系统浏览器的场景。
- **G6 无 macOS 构建机可编译**：Linux CI 可生成 `darwin/amd64` 和 `darwin/arm64` 主程序，并组装相应浏览器资源包，不将本地 macOS / Xcode 作为核心编译前提。macOS 运行验证、正式签名与公证按独立发布步骤落实。

### 2.2 非目标

- 不去浏览器化：使用 chromedp 经 CDP 驱动 Chrome / Edge / Chromium，平台请求继续由页面上下文发出（平台网关强依赖 cookie + `X-Requested-With`）。
- AI 支持两种可选 provider：DeepSeek 网页驱动，以及兼容 OpenAI Chat Completions 的 API；API 配置可指向 DeepSeek 官方 API 或其他兼容服务。
- 不引入 Electron 等桌面外壳：无参数启动在完成 Tempo 登录与当前 AI provider 初始化后打开本地网页面板，由面板完成课堂、实验、关卡范围和模式选择。
- 不扩展题型支持范围。

## 3. 用户与使用场景

| 用户 | 场景 |
| --- | --- |
| 个人用户（学生，Windows / macOS / Ubuntu 桌面环境） | 挂机批量完成课程实验关卡；通过监控面板实时观察题目、AI 输出、评测结果 |
| 同一用户 | 演示 / 试运行（不提交评测）；首次登录（平台 + DeepSeek）；环境自检 |

## 4. 功能需求

### FR-1 配置管理

- 默认在可执行文件所在目录读取 `config.json`；首次缺少配置时由内嵌占位模板生成并提示实际绝对路径，引导填写后退出。配置路径不依赖启动时的工作目录，支持整体搬运。
- 路径优先级：显式 `--config` → 显式 `--data-dir` 中的 `config.json` → 程序同目录的 `config.json`；显式相对路径按工作目录解析。启动成功或配置错误均提示实际使用的配置路径，不回显凭据。
- 启动时使用 `encoding/json` 解码为配置结构体，经集中默认值处理与 `Validate` 校验：必填项缺失、`base_url` 仍为 `YOUR_PLATFORM_HOST` 占位符、字段类型错误，均给出中文明确提示；区分缺省与显式 `false` / `0`，避免覆盖用户配置。
- 缺省值自动补齐：`max_retry=2`、`delay_between_steps=3`、`questions_per_chat=3`、`skip_passed=true`、`cleanup_chat=true`、`answer_timeout=180`。
- **验收**：v1 时代的旧 `config.json` 无缝升级；任何配置错误都不产生裸栈traceback。

### FR-2 登录与会话

- 平台登录：持久化 profile（`data/profile`），页面表单登录；出现图形验证码时提示人工完成。
- DeepSeek 网页 provider 登录：打开对话页由人工完成，`checkLogin()` 检测登录态；选择 API provider 时跳过该登录流程。
- AI API 登录：`ai.provider=openai_compatible` 时不打开 DeepSeek 网页；使用 `ai.base_url`、`ai.model` 与本地 `api_key` 或 `api_key_env` 指定的环境变量。环境变量覆盖配置文件密钥，远端服务只允许 HTTPS；HTTP 仅允许本机地址。API key 不进入日志或流水线产物。
- 浏览器程序查找顺序：当前系统可用的 `Edge → Chrome → 随包 Chromium`，通过明确的可执行文件路径启动，跳过不存在或不支持的候选项。默认完整包必须附带对应系统、架构且经过 CDP 链路验证的 Chromium，系统浏览器不存在或启动失败时自动回退；启动过程不在线下载浏览器，也不要求用户另行安装。
- 自动化复用已安装的浏览器程序，登录态存放于本工具独立 profile，不接管用户日常浏览器 profile。各系统的数据目录见 §6.4。
- 答题期间每 5 分钟检查平台登录态；检测到失效时自动重新登录，必要时等待用户完成验证码。平台会话失效（HTTP 401/302、登录页跳转或明确的过期业务响应）也会自动重登一次再重试；实践环境未启动等普通业务错误不得触发重登。
- DeepSeek 网页 provider 在后台标签生成答案时，无需用户手工切换标签才能继续接收与读取回答。
- 课堂选择：无参数启动的面板显示进行中的课堂名称与班级，用户明确选择后才加载该课堂实验；仅一个课堂时可预选但仍显示课堂名称。终端 `menu` 每次允许重新选择课堂，不写回个人配置。命令行 `--classroom` 优先于 `platform.classroom_id`，所选课堂 ID 贯穿课程、关卡、题目与评测请求；无人值守且存在多个课堂时必须明确指定，禁止默认取首项。
- **验收**：仅安装 Chrome、仅安装 Edge、两者均不可用时，发行包都可正常启动；同一台机器登录一次后跨运行复用，换机器时允许重新登录。

### FR-3 数据浏览（`list` 命令）

- 三类课程（`practice` / `training` / `combat`）→ 章节两层结构拍平为实验列表，编号如 `1-3`。
- 课程与章节的可选 `id` / `coursesId` / `courseId` 允许为 `null`，从有效字段按既有顺序选择实验 ID；实验没有任何可用 ID 时明确报错并停止，不向后续接口传空 ID。
- 关卡列表带完成标记（✓ / ·）；实践环境未启动时给出提示而非报错。
- **验收**：输出格式与 v1 一致。

### FR-4 范围指令解析

- 支持语法：空（全部）/ `第1章` / `第三章` / `第1章 1-5` / `1-3` / `第2章 2` / 名称关键字模糊匹配。
- 中文数字支持到 99（`第二十一章` → 21）。
- 非法输入（如 `1-x`、`第x章`）给出错误提示，不崩溃。
- 交互运行先选课堂，再显示该课堂的实验清单，按列表序号输入单项、连续区间或组合（如 `1-3,5`）；随后按实验逐个列出实际关卡名称、完成状态与列表序号，分别选择关卡范围。各实验独立保存选择，交互中的数字始终指当前列表行号；执行时固定已选关卡 ID，不能把平台关卡编号当作列表序号，也不能将同一范围隐式套用到其他实验。去重、拒绝越界或倒序，`0` 返回、留空重新提示，不默认全部。
- 无参数 GUI 先登录 Tempo，再检测 DeepSeek 登录态；未登录时留在 DeepSeek 标签页并自动等待人工登录，两个登录态都确认后才打开面板。面板显示课堂名称、实验名称和各实验的真实关卡名称及完成状态；可搜索实验、勾选单项，并通过“点击起点、按住 Shift 点击终点”连续选择实验或关卡范围。每个实验的关卡范围独立保存，用户不需要猜序号或输入 CLI 范围。
- GUI 默认不选中全部实验或关卡。完成范围后必须在面板检查课堂、模式、实验和逐关清单并再次确认；取消或清单加载失败时不启动实践环境、AI 请求或评测。正常正式/演示模式跳过已完成关卡，单关试运行必须且只能选择一个真实关卡，允许测试已完成关卡且不提交。
- 终端 `menu` 保留按列表序号输入单项、连续区间或组合（如 `1-3,5`）的交互方式；各实验独立选择实际关卡 ID。显式 CLI `--scope`、`--course`、`--steps` 保留原有筛选语义。
- 正常运行根据 `skip_passed` 排除已完成关卡；若选择范围全部已完成则重新提示，不能无提示地处理零关。关卡列表不可读或为空时明确提示，建议用户检查网络和平台实践环境，不编造编号、不在取消前自动初始化环境；显式 CLI 仍保留确认后准备环境的行为。
- 显式命令行筛选保留原有含义，`--scope` 和 `--course` 取交集；`--yes` / `--headless` 必须明确实验范围，仅给 `--steps` 或 `--limit` 不能默认操作全部实验。
- 交互范围输入兼容中文逗号、顿号；空白、格式错误、越界、一次选择多个课堂分别给出可操作提示。
- **验收**：单测覆盖全部语法分支 + 中文数字边界 + 非法输入降级。

### FR-5 编程题答题（`questionType === "code"`）

- 主链路：`getStep → buildPrompt → ai.ask → 解析 → mergeIntoFramework → evaluate → poll`。
- 多文件题：`===FILE: 路径===` 分节输出与回填；`public class` 名与文件名强制对齐；未按格式输出时降级为只更新第一个文件并告警。
- 单文件题：AI 给完整文件则整体替换；只给片段则合并进 Begin/End 之间（合并前先 dedent，避免双缩进）。
- 命令行类关卡（仅 `README.md` 等）识别并跳过。
- 回答完成度校验：内容特征（代码闭合 / FILE 标记齐全）+ 文本稳定时长，参数沿用 v1。
- **验收**：fixture 对照 v1 输出一致；多文件/单文件/命令行跳过三条路径均有测试。

### FR-6 选择题答题（`questionType === "select"`）

- 支持单选 / 多选 / 判断；字母答案解析（含兜底的顺序抓取）；转换为平台字符串索引格式。
- 未通过时 feedback 带上错题题号，重试上限同 FR-7。
- **验收**：fixture 对照 v1 一致；漏作答触发重试。

### FR-7 失败重试与反馈

- 默认 `max_retry=2`（共 3 次尝试）。
- `diffHint` 精确定位首个差异字符（含「实际输出少了/多了结尾」分支）。
- **快照/副本分离（A1 修复）**：`StepSnapshot` 由未导出字段封装原始题目，仅用于生成 prompt；`Submission` 从快照深拷贝后独立修改，仅用于写回平台。切片、map 和嵌套字段均不得共享可变引用，不能将 Go 结构体值拷贝等同于深拷贝；任何一轮重试的 prompt 中「待补全代码」必须与首次完全一致。
- **验收**：专门的回归测试断言多轮重试的 prompt 中代码框架不被污染。

### FR-8 对话管理

- 单对话连续作答，每满 `questions_per_chat` 题回收当前对话并新开。
- 网页模式只登记发题后实际出现且不属于运行前历史的会话 ID；程序退出时删除登记的会话，不删除其他新建或历史对话。GUI 每段范围完成后保持面板和程序打开，用户可继续选择范围；点击“退出程序”或 Ctrl+C 时统一清理本次运行创建的会话后退出。`cleanup_chat=false` 时不自动删除。
- **用显式 `Captured` 状态区分「未成功获取前置对话列表」与「成功获取但列表为空」**（A2 修复），无法确认归属的会话不得删除。API provider 不创建网页会话，不执行 DeepSeek 对话清理。
- **验收**：账号原本无任何对话时，程序退出时仍能正确清理本次产生的对话。

### FR-9 监控面板

- 默认流程在 Tempo 登录与 AI provider 初始化后打开本地 HTTP 面板（`127.0.0.1:8877` 起，占用则顺延 12 个端口），面板先提供课堂、实验、关卡范围和模式选择，再提供三栏运行监控（题目原文 / AI 输出 / 最终代码与评测结果）、底部日志和顶部进度。每段范围完成后程序保持运行，用户可继续选择新的范围；点击退出按钮时统一执行配置的清理并关闭程序。
- 面板范围页按需读取所选课堂的实验和所选实验的关卡；显示真实名称、完成状态与加载错误，不显示需要用户猜测的平台内部编号。列表支持搜索、复选和 Shift 连选；范围为空、只含正式模式会跳过的已完成关卡或试运行选项不止一关时，不能提交开始请求。
- 启动成功后通过系统浏览器打开面板，优先复用本机 Chrome / Edge；没有可用系统浏览器时使用随包 Chromium，无需额外下载。范围选择 API 仅绑定 loopback，使用随机会话密钥和同源校验；后端只接受本次登录读取并缓存的课堂、实验、关卡 ID。`--headless` 与显式 CLI `run` 保留原行为。
- solver 通过类型化事件推送状态，**不直接 import dashboard 模块**；logger 同为订阅方。
- `/state` 接口 JSON 契约与 v1 完全一致（`view.html` 原样搬迁）。
- **验收**：运行监控 `/state` 与 v1 契约一致；范围页 API 校验来源、会话密钥、重复/空范围和单关试运行约束；无 solver 时 dashboard 可独立启动/测试。

### FR-10 运行模式

- `demo`：假 AI + 强制不提交（互斥优先级最高）。
- `dry-run`：真 AI + 不提交。
- 真实 AI 由 `ai.provider` 选择 DeepSeek 网页或兼容 Chat Completions API；每个 API 题目独立请求，`ai-test` 验证 API 连接。
- `headless`：后台运行，不自动打开面板标签页。
- 面板提供“正式答题”“单关试运行”“流程演示”三种模式。每段范围执行完成后，面板提供继续选择范围和退出程序两个选项；继续时刷新课堂、实验和关卡清单，清空上一段的选择与运行状态，并保留面板和登录会话。单关试运行仅允许选择一个实验、一个关卡，真实 AI、强制不提交，允许选择已完成关卡，仅在本次内存配置中关闭跳过完成关卡；退出该模式后不能影响正式运行与演示模式的配置或数量上限。终端 `menu` 保留同等入口，现有 CLI `--dry-run` 仍按配置中的 `skip_passed` 执行。
- 参数：`--classroom` / `--scope` / `--course` / `--steps` / `--limit` / `--demo` / `--dry-run` / `--headless`。
- 运行前在 GUI 确认框或终端计划中列出选定课堂、模式、实验及各实验实际选中的关卡名称供人工核对；显式 CLI 保留关卡编号范围说明（Ctrl+C 可中断，此时未动成绩）。实验或关卡选择取消后，不能启动 AI 请求、实践环境或评测。
- **验收**：模式组合互斥逻辑正确；清单输出与 v1 一致。

### FR-11 命令集

- `login` / `classrooms`（查看可选课堂及 ID）/ `list` / `run` / `ai-test` / `check` / `menu`（终端交互）。无参数启动默认进入登录后范围面板。
- 面板清楚标识正式运行会提交答案、演示使用模拟 AI、试运行不提交；启动时显示版本与实际配置路径。用户可在课堂、实验和关卡范围面板取消或返回修改；开始前显示完整计划。结束后区分“生成答案”与“评测通过”，不能用试运行成功声称平台判分通过。
- 常见故障先显示中文原因和下一步操作，详细错误仅写入脱敏的私有日志并提示路径；控制台日志使用简洁提示，文件日志保留时间与级别。
- **验收**：命令行为与 v1 一致；未知命令有友好提示。

### FR-12 二进制分发与运行环境

- **主要交付物**：按 §6.5 为各系统与架构发布完整发行包，包含纯 Go 可执行文件、已验证的 Chromium 及必要资源；网页面板和占位配置模板用 `go:embed` 内嵌主程序，运行不要求管理员权限或修改系统 PATH。
- Windows 提供 EXE，macOS / Linux 提供原生可执行文件；均支持 `login` / `list` / `run` / `ai-test` / `check` 及既有参数，退出码应保留。完整包附带适合目标系统的启动入口，macOS 可使用 `.command`；`.app` 为后续可选封装，不作为核心交叉编译的前提。
- 默认完整包的环境依赖必须齐全，优先复用系统 Edge / Chrome，失败时使用随包 Chromium。可额外提供仅主程序的轻量包，并明确其需要已安装的浏览器；免装依赖的默认验收使用完整包。
- 允许首次启动将内嵌资源释放到用户可写目录；资源目录与配置、profile、日志分离，应用升级不得覆盖用户数据。具体路径和清理规则见 §6.4。
- 首次启动与后续启动均不得执行依赖安装、安装 Go / Node / Python 或在线下载浏览器；资源的本地读取、释放与校验由程序自动完成。平台登录、抓题、DeepSeek 对话与评测仍需联网。
- 换机器的操作限于获取与目标系统、CPU 架构匹配的发行物、解压或打开、启动程序、填写或导入配置并登录；不得要求手工配置环境变量、安装依赖、下载浏览器或执行安装脚本。各系统分别提供产物；跨机器、系统和架构不保证旧 profile 可直接复用。
- `check` 可在断网状态下检查本地资源、可用浏览器、数据目录写权限及版本信息；远端连通性检查须单独报告，不能因离线导致本地环境检查崩溃。
- 发布产物附带 SHA-256 校验文件及构建清单（应用版本、源码提交、Go 工具链版本、`go.mod` / `go.sum` 摘要、浏览器来源与版本、目标系统与架构），保留分发所需的第三方许可。
- 禁止将个人 `config.json`、profile、cookie、日志、截图、trace 或开发者缓存打入发行包；包内仅包含占位配置模板。
- **验收**：在 §6.5 的每个目标系统与架构的干净环境中，未安装 Go / Python / Node / npm 时，直接使用对应发行包完成启动、命令行参数、面板打开与本地环境自检；覆盖系统浏览器可用及均不可用的场景。断网时仍能完成资源释放、启动本地面板和本地自检；另覆盖中文与空格路径、同机重启后登录态复用、资源释放中断后的重试。Linux 按选定的 Ubuntu 桌面基线验证且不得临时安装缺失库。真实平台与 AI 链路另按 §10 验收。

## 5. 非功能需求

| 编号 | 需求 |
| --- | --- |
| NFR-1 | 行为一致性：迁移期所有纯函数通过 fixture 对照；除 §11.4 明确记录的缺陷修复外，v1 vs v2 输出 diff 为空 |
| NFR-2 | 健壮性：配置、范围指令、平台返回、AI 输出全部经过校验或降级，无裸 traceback |
| NFR-3 | 可测试性：`scope` / `prompts` / `codemerge` / `config` 语句覆盖率分别 ≥ 90%，关键分支表驱动用例齐全；engine 用 fake provider 测重试逻辑 |
| NFR-4 | 性能：不低于 v1（全程串行 IO-bound，无回归即可） |
| NFR-5 | 安全：`config.json`、`data/`、`logs/`、`.venv/` 进 `.gitignore`；日志不记录凭据与 token |
| NFR-6 | 可维护性：领域模型与平台返回使用明确的 Go 类型，动态 JSON 仅在边界使用 `json.RawMessage` 后校验收敛；索引、类型与错误显式检查；依赖单向：`core ← platform / ai ← solver ← cmd` |
| NFR-7 | 兼容：按 §6.5 的 Windows / macOS / Linux 与 CPU 架构矩阵；Go 工具链和浏览器版本须覆盖最低 OS；默认完整包内含 Chromium，可优先复用系统 Edge/Chrome |
| NFR-8 | 分发可复现：锁定 Go 工具链与模块依赖版本；主程序 `CGO_ENABLED=0`，Linux CI 交叉构建全部目标；构建记录工具链、浏览器与模块摘要，各目标的运行及签名验证单独记录 |

## 6. 架构设计

### 6.1 模块结构

```
cmd/tempo-ai-answer/
└─ main.go             CLI 入口 + 依赖组装
internal/
├─ config/             结构体解码、默认值、校验与模板内嵌
├─ scope/              范围指令解析（纯函数）
├─ core/               领域模型、类型化事件、PlatformError / AIError
├─ browser/            浏览器查找、CDP 会话、独立 profile、平台路径适配
├─ platform/           页面上下文 fetch + 全部平台 API
├─ ai/                 AIProvider 接口、DeepSeek 网页驱动、DemoProvider
├─ prompts/            纯函数：编程题 / 选择题 prompt
├─ codemerge/          纯函数：提取、分节、合并、差异提示
├─ solver/             重试循环、计划执行、快照与提交副本分离
├─ dashboard/
│  ├─ server.go        net/http 服务
│  └─ view.html        go:embed 内嵌，前端交互契约保持一致
└─ logger/             slog 控制台与文件输出、脱敏
testdata/              脱敏 fixture 与 v1 预期输出；测试与 Go 包同目录
scripts/               Linux 交叉构建、浏览器资源组装、校验与发布脚本
packaging/             各系统启动入口、可选 app 封装配置（不包含二进制）
go.mod / go.sum        锁定 Go 版本约束与模块依赖
```

### 6.2 核心设计决策

| 决策 | 说明 |
| --- | --- |
| D1 快照/副本分离 | `StepSnapshot` 隐藏原始可变字段、仅喂 prompt；`Submission` 深拷贝、仅写回平台。封装与别名回归测试共同保证 A1 修复 |
| D2 事件总线 | solver 只发类型化事件，dashboard / logger 订阅。视图可替换、可无视图测试 |
| D3 `AIProvider` 接口 | `Ask / ReadLatest / BeginQuestion / Recycle / Cleanup`；将来切官方 API 只换实现，避免跨层访问私有方法 |
| D4 `PlatformClient` 持有页面会话 | CDP 页面会话封装在 client 内，所有平台请求在页面上下文内 `fetch` 发出并携带 cookie，禁止改为 Go HTTP 客户端直接访问平台 |
| D5 浏览器回退链 | 按当前系统查找 Edge / Chrome 的可执行文件，再尝试随包 Chromium，失败原因进入诊断日志 |
| D6 配置单一来源 | `config` 包提供经过默认值处理与校验的 `Config`，以参数传递，删除全局配置变量模式 |
| D7 选择器集中 | 所有 DOM 选择器集中于 provider 配置对象，平台改版时单点维护 |
| D8 自带依赖与浏览器回退 | Go 主程序内含运行时和网页资源，完整包携带 Chromium；自动化和面板均可使用系统或随包浏览器 |
| D9 资源与用户数据分离 | 配置默认随程序便携；profile、日志存放于系统用户目录。升级替换资源与程序时保留个人配置，资源清理不触及用户数据 |
| D10 纯 Go 交叉构建 | `CGO_ENABLED=0`，通过 `GOOS` / `GOARCH` 在 Linux 生成各目标主程序；不引入需要本地 SDK 的 GUI 或 C/C++ 绑定；各目标独立验证 |
| D11 直接使用 CDP | 采用 chromedp 直接连接浏览器，不需要独立 WebDriver 或 Node 驱动进程；原 Playwright 页面操作需移植与回归，避免改用仍携带外部运行时的封装 |

### 6.3 主数据流

```
CLI 解析参数 → LoadConfig（默认值 + Validate）
  → PlatformClient 启动浏览器 + 登录（可执行程序路径回退）
  → selectClassroom（唯一课堂自动选择，多课堂明确选择，透传 plat.classroomId）
  → collectPlan（实验与关卡范围选择，列出清单确认）
  → 逐实验：prepareCourse（起容器，轮询就绪）
    → 逐关卡：getStep → 分题型
      → select:  buildSelectPrompt → ask → 解析字母 → evaluateSelect → 反馈重试
      → code:    buildPrompt(StepSnapshot) → ask → 完成度校验 → 解析合并(Submission)
                 → evaluate → poll → diff 反馈重试（prompt 始终基于原始快照）
  → 结束：CleanupSince（Captured 状态）→ 删除本次对话
全程：solver emit 事件 → dashboard / logger 消费
```

### 6.4 跨平台打包与资源布局

- **基线路线**：Go + chromedp + `net/http` + `go:embed`。Go 主程序可单文件运行，监控面板和占位模板直接内嵌；Chromium 作为已构建的平台资源纳入完整包，浏览器自身的文件布局与签名保持完整。
- **构建环境**：以 Linux CI 为主要构建环境，显式 `CGO_ENABLED=0`，生成 `windows/amd64`、`darwin/amd64`、`darwin/arm64`、`linux/amd64`。依赖审查和交叉构建必须阻止 CGO 或本地 SDK 依赖进入生产主程序。
- **浏览器组装**：Linux 构建机获取各系统已编译的 Chromium 资源，锁定来源、版本、架构和摘要，核对许可后组装；无需在本地重新编译 macOS 浏览器。包内资源必须完整，不能在用户首次运行时补下载。
- **单文件边界**：主程序的单文件不包含可直接运行的跨平台浏览器。完整包包含主程序与平台浏览器；若进一步制作含浏览器的自解压单文件，需验证权限、资源释放、签名与失败重试，不将其作为首版前置条件。
- **早期验证**：M0.5 先证明 Linux 构建机可产生全部目标二进制，再对可用目标验证系统浏览器探测、随包 Chromium 回退、独立 profile、一次页面上下文请求和内嵌面板。分别记录交叉编译、资源组装、原生运行、真实业务及签名状态；没有 macOS 测试资源时保留待验证状态，不能由构建成功推断可用。
- **用户数据**：配置默认使用程序同目录的 `config.json`，首次生成后提示绝对路径并退出；显式 `--config` 优先，显式 `--data-dir` 次之，不静默回退到系统目录中的旧配置。profile 与日志默认位于系统用户配置目录下的 `tempo-ai-answer`：Windows `%AppData%\tempo-ai-answer\`、macOS `~/Library/Application Support/tempo-ai-answer/`、Linux `${XDG_CONFIG_HOME:-~/.config}/tempo-ai-answer/`。`--data-dir` 可改变其位置；旧版配置可通过 `--config` 读取，不自动覆盖或删除个人配置、profile 或 v1 数据。
- **资源位置**：面板和模板由 `embed.FS` 读取，浏览器从发行包自身资源目录定位，可选 macOS `.app` 内资源保持只读。需要释放资源时使用系统用户缓存目录下的版本与构建摘要子目录，校验完成后才启用，中断后可重试；清理仅限本工具旧版本资源。
- **路径与系统适配**：资源目录、数据目录、浏览器定位、终端与面板打开方式集中封装；使用 `os.Executable`、`filepath` 等定位，不依赖当前工作目录、系统 PATH 中的语言运行时或开发者浏览器缓存。
- **Linux 基线**：首版面向 §6.5 的 Ubuntu 桌面系统，收集并随包提供可再分发的必要共享库；glibc、图形会话、内核及驱动等按声明的 OS 基线验证。不得宣称适用于任意发行版；缺失依赖由构建端解决，不能让用户执行 `apt install`。AppImage 作为可选格式，不能额外要求用户安装 FUSE 才能满足主发行物验收。
- **签名与发布**：Go 交叉编译不等于完成 Developer ID 签名、公证或 macOS 运行验证。正式 macOS 分发另接签名、公证服务或按需 macOS CI，保留浏览器资源及启动脚本的权限；Windows 签名按发行策略落实。不能以让用户关闭系统保护作为正常启动步骤，也不能把未签名产物写成已满足首次启动验收。
- **开发环境**：使用受支持的 Go 稳定版本，M0 锁定具体工具链和依赖；`go.mod` / `go.sum` 同步维护，执行 `gofmt`、`go vet ./...`、`go test ./...`、`go build ./...`；并发编排变更在支持 race detector 的宿主环境补跑竞态检查。终端用户无需安装 Go 或执行这些步骤。

### 6.5 首版支持矩阵

用户确认按 Windows 10/11、macOS 14 及以上（Intel / Apple Silicon）、Ubuntu 22.04/24.04 桌面版评估，首版产物如下：

| 系统 | 架构 | 主要产物 | 验证要求 |
| --- | --- | --- | --- |
| Windows 11 | x64 | ZIP，含 `windows/amd64` EXE、内嵌面板及浏览器资源 | 无开发环境、无系统浏览器仍可启动 |
| Windows 10 | x64 | 与最低 OS 兼容的 Go 主程序及浏览器完整包 | 工具链、浏览器和 CDP 链路分别验证后标记支持 |
| macOS 14 及以上 | x64（Intel） | 含 `darwin/amd64` 主程序、`.command` 入口及浏览器的压缩包；可选 `.app` | Linux 可交叉编译；运行、签名、公证独立验收 |
| macOS 14 及以上 | arm64（Apple Silicon） | 含 `darwin/arm64` 主程序、`.command` 入口及浏览器的压缩包；可选 `.app` | 原生 arm64 验证，不依赖 Rosetta；签名、公证独立验收 |
| Ubuntu 22.04 / 24.04 桌面版 | x64 | 完整目录 `.tar.gz`，可选 AppImage | 两个版本的干净桌面环境均验证，共享库随包适配 |

- Go / chromedp 路线不再采用 Playwright 的系统要求作为应用边界；实际最低 OS 由选定 Go 工具链、浏览器构建与业务验证共同决定。首版仍保持用户确认的范围，版本与架构扩展需新增验证证据。
- Windows / Linux arm64 以及其他 Linux 发行版暂未纳入首版验收，后续按实际需求扩展；不能将 x64 产物或 macOS arm64 产物视作这些目标的支持证据。
- 每个目标记录 Go 工具链、chromedp、Chromium 与最低 OS 版本；Linux CI 可以完成主程序构建与资源组装，但启动与浏览器链路必须在对应系统和架构验证。缺少 macOS 测试机时可使用后续 macOS CI 或测试者反馈，未完成前标记为待验证。

## 7. 技术选型

| 项 | 选型 | 说明 |
| --- | --- | --- |
| 语言与运行时 | Go 1.26.5（go 1.26.0 / toolchain go1.26.5） | `CGO_ENABLED=0`，运行时内置，目标机器无需安装 Go；索引、错误与取消显式处理 |
| 浏览器自动化 | `chromedp` + Chrome DevTools Protocol | 纯 Go 路线，无独立 WebDriver；移植 v1 页面操作和异步脚本执行，M0.5 验证后锁版本 |
| 配置校验 | `encoding/json` + 配置结构体 + 集中 `Validate` | 默认值补齐、字段类型及范围校验、旧配置兼容 |
| CLI | `cobra` + 现有交互菜单迁移 | 子命令与参数契约保持一致，依赖不得引入 CGO |
| 面板资源与 HTTP | `go:embed` + `net/http` | 资源内嵌主程序，系统浏览器打开页面 |
| 日志 | `log/slog` + 控制台 / 文件 handler | 脱敏、落盘，避免引入额外运行时 |
| 测试 | `testing` + 表驱动用例 + fixture | `go test -coverprofile` 报告语句覆盖；必要场景补充 race 检查 |
| 分发 | Linux CI 纯 Go 交叉构建 + 按平台组装完整包 | 原生二进制内嵌面板；浏览器资源使用预构建产物，签名及目标运行验证另行处理 |
| 浏览器与面板 | 本机 Edge / Chrome 优先 + 随包 Chromium 回退 + 本地网页 | 默认发行包已包含备用浏览器，无需用户另行安装 |

### 7.1 跨平台路线比较

| 路线 | 适配本项目的收益与成本 | 结论 |
| --- | --- | --- |
| Go + chromedp | `CGO_ENABLED=0` 时可用 Linux Go 工具链交叉构建首版全部主程序，网页资源可直接内嵌；需移植 Playwright 操作 | 首选，贴合无 macOS 构建机与免装语言环境的约束 |
| Rust + 原生 CDP 驱动 | 可提供原生程序；macOS 交叉链接通常需准备 Apple SDK / 链接工具链，依赖若含原生代码会增加构建配置 | 暂不选作主线，当前约束下构建准备成本较高 |
| Node / Bun + Playwright | 网页自动化迁移方便，但仍需处理运行时、依赖资源和各平台产物 | 可保留作行为参考，主方案已切换 Go |
| Electron / Tauri | 提供桌面窗口，也引入桌面框架和平台资源的构建分发工作 | 当前采用浏览器面板，暂不引入 |

选择 Go 解决主程序的跨平台构建与运行时分发；浏览器文件、Linux 系统库、macOS 签名与公证、目标系统实际运行仍分别验证。无 macOS 构建机不阻断主程序交叉编译，但在取得 macOS CI 或测试反馈前，不能宣称 macOS 端已验收通过。

## 8. 里程碑计划

| 里程碑 | 内容 | 工期 | 出口条件 |
| --- | --- | --- | --- |
| M0 基建 | Go module、锁工具链与依赖、config、slog、go:embed、基本单测与开发检查 | 验证后评估 | `gofmt`、`go vet`、`go test`、`go build` 通过 |
| M0.5 交叉构建与浏览器验证 | Linux 生成四类目标二进制；chromedp 页面脚本、浏览器资源、Linux 库、面板与签名需求验证 | 验证后评估 | 交叉编译和各平台运行状态分别记录；具备测试环境的目标无需安装环境即可运行，缺少 macOS 测试资源时显式保留待验证项 |
| M1 纯函数层 | `scope` / `prompts` / `codemerge` + 表驱动单测 + fixture 对照 | 验证后评估 | 语句覆盖率 ≥90%，关键分支用例齐全，除 §11.4 缺陷修复外与 v1 一致 |
| M2 平台层 | browser / platform，真实验证 login/list/prepare/evaluate | 验证后评估 | 真实账号跑通主链路 |
| M3 AI 层 | DeepSeek（Ask/轮换/清理）+ DemoProvider | 验证后评估 | `ai-test` 通过，轮换与清理验证 |
| M4 编排层 | solver（重试 + 快照分离 + 事件）、dashboard 搬迁 | 验证后评估 | dry-run 全流程通过 |
| M5 分发与试跑 | 各系统打包脚本、校验清单、干净目标环境验证、真实环境 `--limit 2` 小范围试跑、README 更新、tag `v2.0.0` | 验证后评估 | §10 验收全过 |

M0.5 完成后，根据页面自动化移植量、各目标运行验证资源、Linux 库适配及 macOS 签名需求重新估算。原 Node 方案的工期不沿用；每里程碑保持可运行、可停下，构建完成和平台验收分别记录。

## 9. 风险与缓解

| 风险 | 等级 | 缓解 |
| --- | --- | --- |
| 平台接口 / DOM 改版 | 高 | 选择器集中于 provider 配置；失败路径自动截图 + 保留 trace |
| DeepSeek 页面结构变化 | 高 | 选择器集中；`ReadLatest` 兜底；`ai-test` 命令独立验证 |
| 迁移行为偏差 | 中 | 冻结 JSON fixture 对照 + 每阶段可运行；旧源码通过 `python-v1` tag 查阅 |
| 真实账号无法覆盖全部题型 | 中 | 小范围试跑优先覆盖主路径；命令行类等边界用单测 + 人工确认 |
| 持久化 profile 复用兼容性 | 低 | 迁移后首次重新登录一次 |
| chromedp 移植中的自动等待、异步脚本及页面生命周期差异 | 高 | M0.5 优先验证页面上下文 fetch、登录和流式读取，建立显式超时、取消及等待策略，再迁移业务 |
| 系统 Chrome / Edge 版本或企业策略影响自动化 | 中 | 检测浏览器与启动错误，自动回退到随包的匹配 Chromium；记录回退原因 |
| 资源释放中断或中文路径导致启动失败 | 中 | 集中解析路径、临时释放后校验再启用，在各目标系统覆盖路径和中断场景 |
| Linux 浏览器依赖 / 发行版差异 | 高 | 锁定 Ubuntu 桌面基线，构建端收集可分发依赖，在 22.04/24.04 干净环境验证，无需用户安装库 |
| 新增 CGO / 原生 SDK 依赖破坏 Linux 交叉构建 | 高 | 主程序依赖保持纯 Go，四目标 `CGO_ENABLED=0` 构建作为持续检查 |
| Windows 10 的 Go 工具链或浏览器最低版本不匹配 | 中 | 锁定经验证的组合，分别记录主程序、浏览器和业务链路证据 |
| 缺少 macOS 原生运行、签名或公证环境 | 高 | Linux 先生成候选产物；运行验证使用后续 macOS CI / 测试者，正式签名公证另接发布服务，未完成前不标记已验收 |

## 10. 验收标准（Definition of Done）

- [ ] A1–A6、B / D 类问题全部修复，且有对应回归测试或类型级约束（见 §11.1）
- [ ] 纯函数包语句覆盖率分别 ≥ 90%，关键业务分支表驱动用例齐全；覆盖率报告明确使用 Go 的语句指标
- [ ] fixture 行为对照（v1 vs v2）通过
- [ ] 真实环境完成 ≥1 个实验的完整流程，且至少触发过一次重试
- [ ] `.gitignore`、日志落盘、配置校验全部到位
- [ ] README 更新为 v2 用法；v1 代码 tag 归档
- [ ] M0.5 打包验证通过；§6.5 每个目标都有对应完整发行包，换机使用匹配包后无需安装运行环境或依赖即可启动
- [ ] Linux CI 使用 `CGO_ENABLED=0` 成功生成 `windows/amd64`、`darwin/amd64`、`darwin/arm64`、`linux/amd64` 主程序；构建清单完整
- [ ] 每个目标未安装 Go / Python / Node / npm 且没有可用 Chrome / Edge 时，发行物断网仍可读取或释放资源、用随包 Chromium 打开本地面板并完成本地自检
- [ ] 系统浏览器探测与随包浏览器回退、独立 profile、本地面板打开、中文与空格路径、资源释放与升级保留数据均验证通过
- [ ] Windows 10 兼容组合、macOS 两种架构的原生运行及正式签名公证、Ubuntu 22.04/24.04 无需补装依赖分别验证通过；缺少 macOS 测试资源不能用交叉编译成功代替
- [ ] 发行物的 SHA-256、构建清单、第三方许可齐全，且不含真实配置、会话及日志
- [ ] `agent.md` 开发约束落实；正式发布使用 `v2.0.0` 等语义化 annotated tag

## 11. 附录

### 11.1 v1 缺陷 → v2 设计映射

| v1 问题 | v2 对策 |
| --- | --- |
| A1 框架污染 | D1 快照封装、Submission 深拷贝 + 别名与重试回归测试 |
| A2 首次不清理 | `Captured` 状态区分未获取与成功空列表 + 测试 |
| A3 链接 classroomId | 直接透传 `plat.classroomId` |
| A4 配置 KeyError | 配置结构体 + Validate + 默认值合并 |
| A5 channel 硬编码 | D5 回退链 |
| A6 输入崩溃 / 中文数字 | `scope` 包纯函数 + 表驱动单测 + 友好错误 |
| B1 全局变量 | D6 配置单一来源 |
| B2 完成度逻辑重复 | `codemerge` 包单一实现 |
| B3 私有方法跨层 | D3 接口公开方法 |
| B4 solver 过重 | 拆 `prompts` / `codemerge` / `engine` |
| B5 注释漂移 | 重写时统一（完成度判定参数集中定义） |
| C1 预扫重复请求 | `prepareCourse` 内部缓存一次关卡列表 |
| C2 魔法数 | 全部提为具名常量或配置项 |
| D1 零测试 | Go testing + 表驱动用例 + fixture 对照 |
| D2 无 .gitignore | M0 建立 |
| D3 日志不落盘 | slog 文件 handler |
| D4 死依赖 | 仅引入实际使用的依赖 |
| D5 文档不符 | README 随 v2 重写 |

### 11.2 平台 API 契约（迁移时逐条核对）

网关前缀 `/gateway/tempo-talents/api`，请求必须带 `X-Requested-With: XMLHttpRequest`，在页面上下文内发出：

| 功能 | 方法 | 路径 | 关键参数 | 返回要点 |
| --- | --- | --- | --- | --- |
| 当前用户 | GET | `/tt-common/user/current` | — | `data.name` / `data.code` |
| 我的课堂 | POST | `/tt-classroom/list` | `classroomStatus=started` | `data.classroomListInfo.rows[]` |
| 课程列表 | POST | `/tt-classroom/classroomCourseListDetail` | `classroomId`, `coursesType` | 章节两层结构 |
| 关卡列表 | GET | `/tt-course-challenge/challenge/list` | `courseId`, `classroomId` | `data.rows[]` |
| 环境初始化 | POST | `/tt-course-challenge/environment/initialize` | `courseId` | 异步，需轮询关卡列表 |
| 关卡详情 | GET | `/tt-course-challenge/challenge` | `courseStepId` | `questionInfo.questionCodeList` / `selectOrJudgeList` |
| 保存代码 | POST | `/tt-course-challenge/challenge/save` | `codeJson` | — |
| 启动评测 | POST | `/tt-course-challenge/challenge/code/verification/start` | `courseStepId` | ticket |
| 轮询评测 | GET | `/tt-course-challenge/challenge/code/verification` | `ticket` | `passFlag` / `trialSetList` / `containerPhase` |
| 选择题评测 | POST | `/tt-course-challenge/challenge/select-and-judge/verification` | `courseStepId` | 同步返回 |

### 11.3 fixture 数据规范

- 来源：v1 运行时的 `get_step` / `list_courses` / `list_steps` / 评测返回 JSON。
- 脱敏：清除 `username` / `password` / `name` / `code` / 学号 / 任何 token 与 cookie。
- 用途：`prompts` / `codemerge` / `scope` 的行为对照输入；`engine` 的 fake provider 场景来源。

### 11.4 行为对照中的缺陷修复例外

- 正常输入且不涉及已知缺陷的 fixture，要求 v1 与 v2 输出一致。
- A1 框架污染、A2 空前置对话清理、A6 中文数字扩展等已确认缺陷，分别记录 v1 行为、v2 预期结果及对应缺陷编号，用修复后的明确断言验收；不得为了输出一致保留旧缺陷。
- 未归入已确认缺陷的行为差异仍视为迁移回归，需修复或先更新需求后再接受。

### 11.5 打包方案参考

- [Go 构建命令与环境变量](https://pkg.go.dev/cmd/go)：`GOOS` / `GOARCH` 指定目标，工具链版本与构建参数需锁定。
- [CGO 官方说明](https://pkg.go.dev/cmd/cgo)：CGO 的启用与跨平台 C 工具链要求；主程序明确 `CGO_ENABLED=0`。
- [Go embed](https://pkg.go.dev/embed)：在编译时内嵌网页、模板等静态文件。
- [chromedp](https://github.com/chromedp/chromedp)：Go 直接驱动 Chrome DevTools Protocol，无需独立外部驱动。
- [Rust Apple Darwin 目标](https://doc.rust-lang.org/rustc/platform-support/apple-darwin.html)：用于评估 Apple 目标 SDK、链接及交叉编译工具链要求。
