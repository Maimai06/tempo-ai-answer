# Go v2 迁移与验收记录

## 已实现

- 纯 Go CLI、Cobra 命令与交互菜单，内嵌模板和原版面板 HTML。
- 配置缺省补齐、显式 false/0、中文校验、独立用户目录。
- 中文章节至 99、实验编号、关卡范围及错误输入处理。
- 原题深拷贝快照、编程/选择题提示词、答案解析、Java 类名、文件匹配、Unicode 输出差异。
- chromedp 浏览器回退与持久 profile；页面 fetch 保留 Cookie、网关头和表单协议。
- 平台课程/关卡/实践环境初始化、代码/选择题保存与评测、一次自动重登。
- DeepSeek 页面驱动、流式读取、稳定度判断、对话轮换及带 Captured 状态的清理。
- solver 接口注入、重试、demo/dry-run、只读本地面板、文件/控制台日志脱敏。
- CNB 验证、四平台交叉构建、commit 附件和 GitHub 整仓镜像；测试与构建统一由 CNB 承担。

## v1 行为对照

`testdata/v1-parity.json` 是从 `python-v1` 归档版本的纯函数冻结得到的 28 条对照数据，覆盖原样提示词、HTML 转文本、字母答案解析、代码提取和逐字符 diff。Go 测试直接使用 JSON，不依赖 Python 或真实服务；来源与摘要见 [testdata/README.md](../testdata/README.md)。

当前分支已移除 `bot/`、`main.py`、Python 依赖与启动脚本、旧版使用说明及一次性的 Python fixture 生成脚本。旧源码可从 `python-v1` tag（提交 `f8d9e0c9a2db3a6c24a46e20dee294fc4a4db884`）查阅，用户配置和 profile 不参与源码清理。

明确保留的修复差异：

1. 每轮 prompt 和提交均从原始快照独立复制，修复框架污染。
2. Captured=true 且历史为空时仍清理新对话；捕获失败不删除历史。
3. dry-run 同时保护选择题和编程题，demo 优先；缺失/越界选项不提交。
4. 中文数字扩展至 99；倒序、负数及错误范围返回中文错误。
5. 片段合并先 dedent，修复重复缩进；同名文件歧义拒绝随机匹配。
6. public class 对齐仅作用于 Java 文件。
7. 选择题稳定文本可结束等待；代码不完整则不提交。退出时用独立、有限时长的清理上下文。
8. 课堂链接直接使用运行期 Client.ClassroomID，不依赖配置写盘。

## 迁移期间的跨系统回归记录

迁移期间曾使用临时 GitHub Actions 矩阵验证 Windows、macOS Intel/Apple Silicon 与两个 Ubuntu 版本。Ubuntu 24.04 的首次测试暴露浏览器强制退出后仍写 profile 的竞争；改为 CDP 正常关闭并等待分配器回收，回归测试要求退出后能立即删除测试 profile。历史结果仅对应当时提交。现已移除 `.github` 工作流，由 CNB 统一执行测试和交叉构建，后续各系统原生运行验证单独记录。

## 验收边界

本地自动验证包含：四纯包各 ≥90% 语句覆盖率、fake provider 重试/不提交/取消、快照嵌套引用隔离、对话清理边界、面板并发与端口回退、协议 fixture、真实浏览器 loopback CDP、Git 镜像临时裸库回归、race/vet/build、四目标交叉编译。

自动化不能证明以下项目，首个正式 v2 版本前仍需完成：

- 真实 Tempo 平台端到端登录、验证码、实践环境、编程及选择题评测。
- 真实 DeepSeek 页面选择器、长回复、侧栏删除及长期登录态复用。
- Windows 10/11、macOS 14+ Intel/Apple Silicon、Ubuntu 22.04/24.04 干净桌面环境。
- 无系统浏览器时的完整包：浏览器版本/摘要锁、许可证、Linux 必要库、离线启动、中文/空格路径和资源恢复。
- macOS 签名、公证及系统隔离属性；跨编译不代替这些验证。
- CNB 实际流水线、密钥访问授权和 GitHub 镜像成功状态，以远端运行记录为准。

在上述验收前不打正式 v2 tag，不宣称已实现全部系统免依赖开箱运行。
