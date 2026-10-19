# v1 行为对照数据

`v1-parity.json` 是迁移时从原 Python 纯函数冻结的输入与预期结果。旧源码归档于 `python-v1` tag，对应提交 `f8d9e0c9a2db3a6c24a46e20dee294fc4a4db884`。当前分支已移除 Python 源码和一次性生成脚本，Go 测试直接读取这些 JSON，不需要 Python。

共 28 条数据：编程题提示词 8 条、选择题提示词 2 条、HTML 转文本 3 条、选择题答案解析 5 条、代码提取 4 条、输出差异 6 条。

冻结文件的 SHA-256：`020804aabc06093eecd3d24911bee9980fffb2c8e190a51cefbe7718e5e9bb93`。

这些结果用来检测迁移回归，不应根据当前 Go 实现自动重写。已确认缺陷的预期差异在 `docs/MIGRATION.md` 单独记录和验证。
