# Markdown Tools

适合日常写 README、项目笔记和待办清单。需要 Python 3.9+，只使用标准库；分屏格式化功能需要支持 `preview.markdown` 的新版 SunBook。

安装整个 `markdown-tools` 文件夹，审核后启用。打开 Markdown 文档，按 F1：

| 命令 | 用途 |
| --- | --- |
| **Markdown Tools: Format Markdown in Split Preview** | 在原文件右侧打开实时预览，左侧继续编辑；右侧 Format 按钮单独应用格式化，不新建标签 |
| **Markdown Tools: Update Table of Contents** | 生成或更新二至六级标题目录，保留正文；通常放在首个一级标题后，无标题时放在正文开头 |
| **Markdown Tools: Show Task Progress** | 显示完成数量、完成率、剩余任务和行号，不修改文档 |
| **Markdown Tools: Toggle Selected Tasks** | 切换光标所在任务或选中行中的 `[ ]` / `[x]`；支持反向、多选区和大写 `[X]` |

目录位于 `<!-- sunbook:toc -->` 与 `<!-- /sunbook:toc -->` 标记之间，重复执行不会重复插入。已存在标记时仅替换该区域；删掉全部二至六级标题后，会清空已有目录。标记不匹配时返回说明，不猜测删除范围。

支持 `#` 风格标题和单行 Setext 标题，跳过 YAML front matter、围栏代码块、HTML 注释及普通缩进代码；支持常见嵌套和有序任务列表。目录链接保留 Unicode 字母、数字、组合标记、连字符和下划线，空白变为连字符、重复锚点追加序号；常见行内代码、链接、粗体和 HTML 标签会转为标题文本。复杂 Markdown 扩展、自定义锚点、块引用中的标题/任务、多行 Setext 标题不在本插件解析范围内，不保证所有渲染器的自定义锚点规则一致。

任务进度最多列出前 100 个未完成任务，较长任务文字只在报告中截短，原文保持不变。支持最多 1 MiB 的文档、500 个目录条目；切换任务最多处理 256 个选区。所有位置使用 UTF-8 字节偏移，中文、Emoji、组合字符不会按 Python 字符下标错误修改。

权限仅有 `document.read` 和 `document.edit`：前者用于分析当前不可变文档快照，后者用于返回目录/任务修改。所有修改经过 SunBook 预览确认，保持未保存状态，并可以一次撤销。不会读写文档文件、扫描工作区或发起网络请求；macOS 仍使用默认禁网沙箱和独立临时 HOME，不需要凭据或持久化数据。

无文档、未闭合元数据、损坏的目录标记、超出大小限制会返回普通命令错误，进程继续响应健康检查。没有标题/任务、目录无需更新、光标不在任务上时返回操作说明。

开发验证（在仓库根目录）：

```sh
./sunbook --check-plugin examples/markdown-tools
python3 -B -m unittest discover -s examples/markdown-tools -p 'test_*.py'
go test -race ./internal/ui -run TestMarkdownPluginInstallCommandsPreviewAndUndo -v
python3 scripts/smoke_markdown_plugin.py ./sunbook
make check
make smoke
```

`make smoke` 会发现本目录的命令测试，并运行独立 PTY 安装测试；Go 的示例兼容性检查会验证本插件握手与健康协议。`sunbook_plugin.py` 与仓库 Python SDK 同版本，CI 检查副本一致性。

插件专项 PTY 测试默认使用临时配置。显式提供 `--config /absolute/path/to/config.json` 时，会安装、审核、启用并保留插件，保留配置中的其他内容。默认拒绝覆盖已安装的同名插件；额外传 `--replace` 时先将旧包备份到配置目录的 `plugin-backups/`，再通过编辑器禁用、移除、安装和审核新版。

格式化规则包括标题空格及前后空行、连续空行、常见顶层无序列表标记、大写复选框、管道表格列排版及文件末尾换行；保留段落换行、硬换行、代码块、HTML 区块、元数据和缩进代码。不会将 `#title` 这类普通文本强行改成标题，不重排长段落；代码围栏或元数据未闭合时仍可打开实时预览，Format 会提示补全后再格式化。表格最多 32 列/1000 行，列宽超过 120 字符时原样保留。分屏左侧是原文件的实际编辑器，可直接输入、粘贴、保存和撤销；右侧默认阅读预览，读取未保存的内容，在约 100ms 输入停顿后后台更新，不新建标签。中线可拖动，两侧独立滚动。右侧 **Format**（或 F1 → **Plugins: Format Preview Document**）格式化当前内容，修改保持未保存并可一次撤销；格式化请求期间继续输入会取消过期修改。**Source/Read** 切换当前 Markdown 源码/阅读，**Close** 关闭右侧。在右侧获得焦点后，可用 `a` / Ctrl+Enter 格式化、`v` 切换、Tab 回编辑器、Esc 关闭；左侧的 `a`、`v` 是普通输入。终端阅读预览支持标题、粗体/斜体、代码、引用、任务和表格，省略原始 HTML，不加载图片或远程资源；不是浏览器 HTML 预览。保留普通编辑器已有分屏布局，关闭预览后恢复。切换文件时隐藏预览，回到原文件恢复。

版本变更见 [CHANGELOG.md](CHANGELOG.md)，宿主协议见仓库 [开发规范](../../docs/plugins.md)。
