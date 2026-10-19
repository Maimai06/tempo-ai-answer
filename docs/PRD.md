# Tempo AI 自动答题助手 v2 · 产品需求文档（PRD）

| 项 | 内容 |
| --- | --- |
| 产品 | Tempo AI 自动答题助手（美林 Tempo 实训平台） |
| 版本 | v2.0 —— Node/TypeScript 重写版 |
| 日期 | 2026-10-19 |
| 状态 | 草案 |
| 上游 | v1（Python + Playwright，已归档保留作对照基准） |

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

- **G1 技术栈迁移**：Python → Node 20 LTS + TypeScript（strict）+ Playwright，功能与 v1 完全对齐。
- **G2 债务清零**：A / B / C / D 全部问题在新设计中消除（见 §11.1 映射表）。
- **G3 可测试**：纯函数 100% 分支覆盖；迁移期用 fixture 做 v1↔v2 行为对照。
- **G4 行为一致性**：同一题目输入下，v2 与 v1 的 prompt 生成、代码提取、多文件合并、答案解析输出一致。

### 2.2 非目标

- 不去浏览器化：继续使用 Playwright + 页面上下文请求（平台网关强依赖 cookie + `X-Requested-With`）。
- 不更换 AI 供应商：仍驱动网页版 DeepSeek，但保留 `AiProvider` 接口，为将来切换官方 API 预留。
- 不做桌面 GUI：监控面板维持本地网页形态。
- 不扩展题型支持范围。

## 3. 用户与使用场景

| 用户 | 场景 |
| --- | --- |
| 个人用户（学生，Windows 10/11） | 挂机批量完成课程实验关卡；通过监控面板实时观察题目、AI 输出、评测结果 |
| 同一用户 | 演示 / 试运行（不提交评测）；首次登录（平台 + DeepSeek）；环境自检 |

## 4. 功能需求

### FR-1 配置管理

- 首次运行自动从 `config.example.json` 生成 `config.json`，引导填写后退出。
- 启动时经 zod 校验：必填项缺失、`base_url` 仍为 `YOUR_PLATFORM_HOST` 占位符、字段类型错误，均给出中文明确提示。
- 缺省值自动补齐：`max_retry=2`、`delay_between_steps=3`、`questions_per_chat=3`、`skip_passed=true`、`cleanup_chat=true`、`answer_timeout=180`。
- **验收**：v1 时代的旧 `config.json` 无缝升级；任何配置错误都不产生裸栈traceback。

### FR-2 登录与会话

- 平台登录：持久化 profile（`data/profile`），页面表单登录；出现图形验证码时提示人工完成。
- DeepSeek 登录：打开对话页由人工完成，`checkLogin()` 检测登录态。
- 浏览器启动 channel 回退链：`msedge → chrome → bundled chromium`。
- 平台会话失效（HTTP 401/302）自动重登一次再重试。
- **验收**：仅安装 Chrome 的机器可正常启动；登录一次后跨运行复用。

### FR-3 数据浏览（`list` 命令）

- 三类课程（`practice` / `training` / `combat`）→ 章节两层结构拍平为实验列表，编号如 `1-3`。
- 关卡列表带完成标记（✓ / ·）；实践环境未启动时给出提示而非报错。
- **验收**：输出格式与 v1 一致。

### FR-4 范围指令解析

- 支持语法：空（全部）/ `第1章` / `第三章` / `第1章 1-5` / `1-3` / `第2章 2` / 名称关键字模糊匹配。
- 中文数字支持到 99（`第二十一章` → 21）。
- 非法输入（如 `1-x`、`第x章`）给出错误提示，不崩溃。
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
- **快照/副本分离（A1 修复）**：`StepSnapshot`（只读原始题目）仅用于生成 prompt，`Submission`（可变副本）仅用于写回平台；任何一轮重试的 prompt 中「待补全代码」必须与首次完全一致。
- **验收**：专门的回归测试断言多轮重试的 prompt 中代码框架不被污染。

### FR-8 对话管理

- 单对话连续作答，每满 `questions_per_chat` 题回收当前对话并新开。
- 运行结束删除本次新产生的对话；**用 `undefined` 哨兵区分「未获取前置对话列表」与「前置列表为空」**（A2 修复）。
- **验收**：账号原本无任何对话时，运行结束后仍能正确清理本次产生的对话。

### FR-9 监控面板

- 本地 HTTP 服务（`127.0.0.1:8877` 起，占用则顺延 12 个端口），三栏布局（题目原文 / AI 输出 / 最终代码与评测结果）+ 底部运行日志 + 顶部进度与状态。
- solver 通过类型化事件推送状态，**不直接 import dashboard 模块**；logger 同为订阅方。
- `/state` 接口 JSON 契约与 v1 完全一致（`view.html` 原样搬迁）。
- **验收**：面板行为与 v1 一致；无 solver 时 dashboard 可独立启动/测试。

### FR-10 运行模式

- `demo`：假 AI + 强制不提交（互斥优先级最高）。
- `dry-run`：真 AI + 不提交。
- `headless`：后台运行，不自动打开面板标签页。
- 参数：`--scope` / `--course` / `--steps` / `--limit` / `--demo` / `--dry-run` / `--headless`。
- 运行前列出实验清单供人工核对（Ctrl+C 可中断，此时未动成绩）。
- **验收**：模式组合互斥逻辑正确；清单输出与 v1 一致。

### FR-11 命令集

- `login` / `list` / `run` / `ai-test` / `check` / 无参数交互菜单。
- **验收**：命令行为与 v1 一致；未知命令有友好提示。

## 5. 非功能需求

| 编号 | 需求 |
| --- | --- |
| NFR-1 | 行为一致性：迁移期所有纯函数通过 fixture 对照（v1 vs v2 输出 diff 为空） |
| NFR-2 | 健壮性：配置、范围指令、平台返回、AI 输出全部经过校验或降级，无裸 traceback |
| NFR-3 | 可测试性：`scope` / `prompts` / `codemerge` / `config` 分支覆盖率 ≥ 90%；engine 用 fake provider 测重试逻辑 |
| NFR-4 | 性能：不低于 v1（全程串行 IO-bound，无回归即可） |
| NFR-5 | 安全：`config.json`、`data/`、`logs/`、`.venv/` 进 `.gitignore`；日志不记录凭据与 token |
| NFR-6 | 可维护性：`strict` + `noUncheckedIndexedAccess`；平台返回用 `unknown` + guard 收敛，不用 `any` 逃逸；模块依赖单向：`core ← platform / ai ← solver ← index` |
| NFR-7 | 兼容：Windows 10/11 + Edge/Chrome，Node 20 LTS |

## 6. 架构设计

### 6.1 模块结构

```
src/
├─ index.ts            CLI 入口（commander）
├─ config.ts           zod schema + 默认值合并 + 占位符校验
├─ scope.ts            范围指令解析（纯函数）
├─ core/
│  ├─ types.ts         领域模型 + 平台返回类型（含 isTrailSetVisible 拼写注释）
│  ├─ events.ts        类型化 EventEmitter
│  └─ errors.ts        PlatformError / AiError
├─ platform/
│  ├─ browser.ts       持久化 context + channel 回退链
│  └─ client.ts        page context 内 fetch 封装 + 全部平台 API
├─ ai/
│  ├─ provider.ts      AiProvider interface
│  ├─ deepseek.ts      DeepSeek 网页驱动（移植 WebAI）
│  └─ demo.ts          DemoProvider
├─ solver/
│  ├─ prompts.ts       纯函数：buildPrompt / buildSelectPrompt
│  ├─ codemerge.ts     纯函数：extractCode / splitMulti / mergeIntoFramework / diffHint
│  └─ engine.ts        重试循环（快照/副本分离）
├─ dashboard/
│  ├─ server.ts        node:http 服务
│  └─ view.html        前端原样搬迁
└─ util/logger.ts      pino（控制台 + 文件）
tests/                 vitest：单测 + fixture 对照
```

### 6.2 核心设计决策

| 决策 | 说明 |
| --- | --- |
| D1 快照/副本分离 | `StepSnapshot` 只读、仅喂 prompt；`Submission` 可变、仅写回平台。类型层面杜绝 A1 |
| D2 事件总线 | solver 只发类型化事件，dashboard / logger 订阅。视图可替换、可无视图测试 |
| D3 `AiProvider` 接口 | `ask / readLatest / beginQuestion / recycle / cleanup`；将来切官方 API 只换实现，`_read` 私有访问问题随之消失 |
| D4 `PlatformClient` 持有 page | 所有请求在页面上下文内 `fetch` 发出，自动携带 cookie；禁止用 Node fetch 直连平台 |
| D5 channel 回退链 | `msedge → chrome → bundled chromium`，启动失败自动降级 |
| D6 配置单一来源 | `config.ts` 导出强类型 `Config`，全项目无散落下标访问，删除全局变量模式 |
| D7 选择器集中 | 所有 DOM 选择器集中于 provider 配置对象，平台改版时单点维护 |

### 6.3 主数据流

```
CLI 解析参数 → loadConfig（zod 校验）
  → PlatformClient 启动浏览器 + 登录（channel 回退）
  → ensureClassroom（自动选课堂，直接透传 plat.classroomId）
  → collectPlan（范围指令过滤）
  → 逐实验：prepareCourse（起容器，轮询就绪）
    → 逐关卡：getStep → 分题型
      → select:  buildSelectPrompt → ask → 解析字母 → evaluateSelect → 反馈重试
      → code:    buildPrompt(StepSnapshot) → ask → 完成度校验 → 解析合并(Submission)
                 → evaluate → poll → diff 反馈重试（prompt 始终基于原始快照）
  → 结束：cleanupSince（undefined 哨兵）→ 删除本次对话
全程：solver emit 事件 → dashboard / logger 消费
```

## 7. 技术选型

| 项 | 选型 | 说明 |
| --- | --- | --- |
| 运行时 | Node 20 LTS | Windows 10/11 兼容 |
| 语言 | TypeScript `strict` + `noUncheckedIndexedAccess` | 编译期消除 `codes[0]` 类边界缺陷 |
| 浏览器自动化 | `playwright` | 与 v1 API 近似，迁移阻力最小 |
| 配置校验 | `zod` | 默认值合并 + 类型收敛 |
| CLI | `commander` | 子命令结构清晰 |
| 日志 | `pino` | 控制台 + 文件双路 |
| 测试 | `vitest` | 零配置，ESM 友好 |
| 分发 | Node 绿色版 + `.cmd` 启动脚本 | `install.cmd` 执行 `npm ci`；单文件打包（pkg/bun）列为可选项 |

## 8. 里程碑计划

| 里程碑 | 内容 | 工期 | 出口条件 |
| --- | --- | --- | --- |
| M0 基建 | 脚手架、tsconfig strict、`config.ts`（zod）、logger、`.gitignore`、vitest 跑通 | 0.5d | 空项目可 build + test |
| M1 纯函数层 | `scope.ts` / `prompts.ts` / `codemerge.ts` + 单测 + fixture 对照 | 1d | 覆盖率 ≥90%，与 v1 输出 diff 为空 |
| M2 平台层 | `browser.ts` / `client.ts`，真实验证 login/list/prepare/evaluate | 1d | 真实账号跑通主链路 |
| M3 AI 层 | `deepseek.ts`（ask/轮换/清理）+ `demo.ts` | 1d | `ai-test` 通过，轮换与清理验证 |
| M4 编排层 | `engine.ts`（重试 + 快照分离 + 事件）、dashboard 搬迁 | 1d | dry-run 全流程通过 |
| M5 试跑收尾 | 真实环境 `--limit 2` 小范围试跑、README 更新、tag `v2.0` | 0.5d | §10 验收全过 |

合计约 **5 人日**。每里程碑保持可运行、可停下。

## 9. 风险与缓解

| 风险 | 等级 | 缓解 |
| --- | --- | --- |
| 平台接口 / DOM 改版 | 高 | 选择器集中于 provider 配置；失败路径自动截图 + 保留 trace |
| DeepSeek 页面结构变化 | 高 | 选择器集中；`readLatest` 兜底；`ai-test` 命令独立验证 |
| 迁移行为偏差 | 中 | fixture 对照 + 每阶段可运行；v1 保留可跑 |
| 真实账号无法覆盖全部题型 | 中 | 小范围试跑优先覆盖主路径；命令行类等边界用单测 + 人工确认 |
| 持久化 profile 复用兼容性 | 低 | 迁移后首次重新登录一次 |

## 10. 验收标准（Definition of Done）

- [ ] A1–A6、B / D 类问题全部修复，且有对应回归测试或类型级约束（见 §11.1）
- [ ] 纯函数分支覆盖率 ≥ 90%
- [ ] fixture 行为对照（v1 vs v2）通过
- [ ] 真实环境完成 ≥1 个实验的完整流程，且至少触发过一次重试
- [ ] `.gitignore`、日志落盘、配置校验全部到位
- [ ] README 更新为 v2 用法；v1 代码 tag 归档

## 11. 附录

### 11.1 v1 缺陷 → v2 设计映射

| v1 问题 | v2 对策 |
| --- | --- |
| A1 框架污染 | D1 快照/副本类型分离 + 回归测试 |
| A2 首次不清理 | `undefined` 哨兵 + 测试 |
| A3 链接 classroomId | 直接透传 `plat.classroomId` |
| A4 配置 KeyError | zod 校验 + 默认值合并 |
| A5 channel 硬编码 | D5 回退链 |
| A6 输入崩溃 / 中文数字 | `scope.ts` 纯函数 + 单测 + 友好错误 |
| B1 全局变量 | D6 配置单一来源 |
| B2 完成度逻辑重复 | `codemerge.ts` 单一实现 |
| B3 私有方法跨层 | D3 接口公开方法 |
| B4 solver 过重 | 拆 `prompts` / `codemerge` / `engine` |
| B5 注释漂移 | 重写时统一（完成度判定参数集中定义） |
| C1 预扫重复请求 | `prepareCourse` 内部缓存一次关卡列表 |
| C2 魔法数 | 全部提为具名常量或配置项 |
| D1 零测试 | vitest + fixture 对照 |
| D2 无 .gitignore | M0 建立 |
| D3 日志不落盘 | pino 文件 sink |
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
