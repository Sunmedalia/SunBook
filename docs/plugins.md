# SunBook 插件开发规范（API v1）

SunBook 插件是通过标准输入、标准输出通信的独立进程，适合扩展编辑命令、文档分析和文本转换。宿主提供超时、重启退避、健康检查和熔断。macOS 默认要求操作系统沙箱，限制文件访问并默认禁网；Linux 当前提供进程故障隔离，文件和网络访问仍是当前用户权限。两端都会过滤宿主环境变量并提供独立临时 HOME。启用前请阅读界面显示的保护范围，只运行可信代码。

## 安装与使用

1. 在左侧 **Plugins**（拼图图标）选择 **Install…**，输入插件文件夹路径。也可以使用 `F1 → Plugins: Install Local Plugin…`。
2. 安装包复制到当前配置文件旁的 `plugins/<id>/`，安装后保持禁用。自定义 `--config` 会使用对应配置目录。
3. 选中插件按 Enter，选择 **Enable plugin…**，阅读文档访问权限并启用。
4. 在插件详情或 F1 命令面板运行命令。文本修改必须经过预览确认，保留为未保存修改，可一次撤销。
5. 在详情中禁用、重启、查看健康和最近 stderr，或先禁用再移除本地副本。

试用仓库自带的 [Text Tools](../examples/text-tools)：安装 `examples/text-tools` 文件夹，启用后可运行 **Text Tools: Count Words** 和 **Text Tools: Uppercase Selection**。后者支持多个选区、中文和 Emoji。

手动安装也可将完整插件目录复制到配置目录的 `plugins/`，然后执行 **Plugins: Refresh Installed Plugins**；运行中也每 30 秒检查一次插件目录。批准覆盖整个插件包的路径、文件内容和文件可执行位，清单或代码变更均停止旧进程并要求重新审核；每次启动前也会重新校验。安装过程中源内容发生变化会拒绝安装。旧版仅针对清单的批准记录升级后需要重新审核一次。当前没有在线市场或自动下载、自动更新。

## 插件目录与清单

目录必须与 `id` 同名。安装及手动放入的插件包只允许普通文件和目录，不接受符号链接或特殊文件；上限为 1000 个文件、64MiB、2000 个文件和目录条目（含根目录）。清单上限为 64KiB。运行时生成的缓存或状态请写入宿主提供的临时 HOME/TMPDIR，不要修改安装目录。

```text
text-tools/
  plugin.json
  main.py
  sunbook_plugin.py
```

```json
{
  "id": "text-tools",
  "name": "Text Tools",
  "version": "1.0.0",
  "apiVersion": 1,
  "description": "Count words and transform selected text.",
  "command": ["python3", "main.py"],
  "permissions": ["document.read", "document.edit"],
  "commands": [
    {"id": "word-count", "title": "Count Words"},
    {"id": "uppercase-selection", "title": "Uppercase Selection"}
  ]
}
```

清单约束以 [manifest-v1.schema.json](plugins/manifest-v1.schema.json) 和宿主 `ReadManifest` 校验为准。ID 使用小写字母、数字、点和连字符，长度不超过 64；命令 ID 在插件内唯一。宿主快捷键 ID 为 `plugin.<插件ID>/<命令ID>`，分隔符 `/` 不允许出现在插件或命令 ID 中，因此命名空间不会冲突。可在配置中使用 `"keybindings": {"ctrl+alt+w": "plugin.text-tools/word-count"}` 为插件命令绑定快捷键。版本使用 `X.Y.Z`，可附加 `-suffix`。目前只接受 `apiVersion: 1`，未知字段、未知权限和不兼容 API 会标记为 Invalid。

`command` 是直接执行的 argv 数组，不经过 Shell。程序从 PATH 查找，或使用相对插件目录的 `./program`、绝对路径；工作目录始终是插件目录。不要依赖编辑器启动目录。Python、Node 等运行时由用户安装。插件包批准不包含外部运行时或启动程序的发行方校验；请自行维护其可信来源。macOS 支持系统/Homebrew Python 和直接指定解释器路径的 pyenv/uv/virtualenv，不继承 Shell 初始化设置或自动执行版本管理器 shim；依赖这些设置的命令请改为实际解释器路径。

文档权限约束宿主提供的数据和操作；`network` 在 macOS 上控制网络访问：

| 权限 | 宿主行为 |
| --- | --- |
| 未声明权限 | 不向插件提供文档，包括文件路径和内容 |
| `document.read` | 提供当前文档的不可变快照、版本及选区 |
| `document.edit` | 允许返回当前快照的文本修改；必须同时声明 `document.read` |
| `network` | macOS 允许网络连接（包括本机服务）；必须在清单中声明并重新审核。Linux 当前无网络沙箱，无论是否声明均可联网 |

### macOS 文件与网络沙箱

宿主通过 `/usr/bin/sandbox-exec` 启动插件，默认拒绝未列入规则的操作。允许读取自身插件包、系统与运行时文件，允许在独立临时目录写入文件；不能读取普通工作区/用户文档、修改安装目录或其他用户文件。编辑中的文档只能按 `document.read` 通过协议快照提供。插件及其正常子进程继承同一套限制，默认禁止 TCP、UDP 和 Unix socket 访问；需要网络的插件显式声明 `network`。这项权限也允许访问本机网络服务，仍需审核插件用途。

只读运行时范围包括 `/usr`（含 `/usr/local`）、`/bin`、`/sbin`、`/opt/homebrew`、系统库和 Cryptex OS 镜像、`/Library/Apple`、`/Library/Frameworks`、`/Library/Developer`、Xcode 开发工具、系统 SSL 证书与时区数据，以及插件的启动可执行文件。不会放开整个 `/System` 或 `/Library`，尤其不会通过 `/System/Volumes/Data` 别名开放用户数据。自定义 CPython/pyenv/uv 解释器额外只读其对应的 `lib/pythonX.Y`、`libpythonX.Y*.dylib`；不开放其整个父目录或用户主目录。放在上述运行时目录里的文件也会处于可读范围，勿将私人文档或凭据放在这些目录中。路径元信息可读取，以便运行时解析父目录；不等于可以读取文件内容。

Python 示例默认不生成 `__pycache__`，HOME、TMPDIR、TEMP/TMP 和 XDG 缓存/配置/数据目录均指向每次启动独立的临时目录；退出、禁用或重启后清理，不提供持久化存储。宿主仅传入必要的 PATH、语言、时区和协议环境变量，不继承 API 密钥、SSH agent、代理凭据、PYTHONPATH、NODE_OPTIONS、DYLD 等变量。需要外部服务凭据的插件当前没有宿主凭据接口，请勿通过清单参数写入密钥。

沙箱工具缺失、策略不兼容或受限启动失败时，插件启动失败并显示健康错误，**不会自动降级为无沙箱运行**。Apple 已在本机 `man sandbox-exec` 中将该工具标记为 deprecated；这是当前源码使用的 Seatbelt 后端，不是签名应用的 App Sandbox 发布方案。Apple 的正式应用沙箱与 helper 分发方式见 [App Sandbox](https://developer.apple.com/documentation/security/app-sandbox) 和 [嵌入命令行 helper](https://developer.apple.com/documentation/xcode/embedding-a-helper-tool-in-a-sandboxed-app)。更换 macOS 版本后应重新运行本文的边界测试。当前不承诺硬性内存、CPU、磁盘空间或进程数量配额，也不将本地包指纹当作发行方签名。

Linux 当前没有同等的文件/网络沙箱；环境变量过滤和临时 HOME 不能阻止插件自行访问当前用户可读的凭据或文件。界面会明确显示这一范围，不将其称为操作系统沙箱。

## 传输与协议

- stdin/stdout 使用 UTF-8 的逐行 JSON-RPC 2.0，每条消息占一行，末尾 LF；不得混入日志、调试输出或启动横幅。
- 单条编码后消息最多 4MiB；ID 为宿主生成的正整数，响应必须原样返回。一个进程内请求串行执行。
- 宿主只发出 `initialize`、`ping` 和 `execute`；本版不支持插件主动请求宿主、动态注册命令或任意磁盘编辑。
- 日志写 stderr，宿主仅保留最近 16KiB，并在 stderr 超过每秒 1MiB 时终止并重启插件。错误使用 JSON-RPC `error`，不要为普通命令错误退出进程。

初始化请求：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"host":"SunBook","capabilities":["commands","document.read","document.edit"]}}
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}
```

开发检查命令的初始化请求可能不带可选 `capabilities`。插件必须验证协议版本；不支持的版本返回错误。宿主每 30 秒发送 `ping`，插件应快速响应空对象：

```json
{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}
{"jsonrpc":"2.0","id":2,"result":{}}
```

执行命令（无读取权限时省略 `document`）：

```json
{"jsonrpc":"2.0","id":3,"method":"execute","params":{"command":"uppercase-selection","document":{"path":"/project/main.txt","version":4,"text":"中 hello","selections":[{"anchor":4,"head":9}]}}}
{"jsonrpc":"2.0","id":3,"result":{"title":"Uppercase selection","edits":[{"start":4,"end":9,"text":"HELLO"}]}}
```

所有选区和编辑位置都是 **原始快照中的 UTF-8 字节偏移**，区间为 `[start,end)`，不是 Python 字符下标或 JavaScript UTF-16 下标。允许反向选区；修改必须非重叠、处于完整 Unicode 字素边界，并且不能包含 NUL。快照使用 LF，宿主将返回文本中的 CRLF/CR 统一为 LF；保存时继续保留原文件的换行格式。宿主在后台校验全部修改，确认时再次核对活动文档、缓冲区身份和版本；过期结果整体拒绝，不部分应用。

结果对象可包含 `title`、`message`、`body`、`edits`。无修改时 `body` 在可滚动结果窗口显示，否则显示 `message`；存在修改时进入确认预览。结果不会打断已打开的对话框，可关闭对话框后使用 **Plugins: Show Last Result** 查看最近一次结果。只保留最近一份结果；当前为只读 Git 对比时请先打开工作文件再运行插件。最多 10000 个修改、256KiB body、4096 字节 message、200 字节 title；修改后的文档最多 4MiB。大小以 UTF-8 编码后的字节数计。JSON 转义可能使消息超过传输限制，因此不要假设 4MiB 原始文本一定可以传输。

普通业务错误示例：

```json
{"jsonrpc":"2.0","id":3,"error":{"code":-32000,"message":"Select text before running this command"}}
```

## Python SDK

[sdk/python/sunbook_plugin.py](../sdk/python/sunbook_plugin.py) 只依赖 Python 3.9+ 标准库。将该文件放入插件目录，与插件代码一同发布；示例插件包含同版本副本，CI 校验两者一致。

```python
from sunbook_plugin import Plugin, PluginError, selected_text


def count(document):
    if document is None:
        raise PluginError("Open a document first")
    return {"body": f"Words: {len(document['text'].split())}"}


Plugin({"word-count": count}).run()
```

`selected_text(document, selection)` 返回选中文本及起止字节偏移，可正确处理 Python 字符下标与 UTF-8 的差异。SDK 处理初始化、健康检查、请求解析、业务异常和标准输出刷新。不要在处理器中调用 `print()` 输出日志；使用 `print(..., file=sys.stderr)`。

## 可用性与故障策略

| 情况 | 宿主处理 |
| --- | --- |
| 初始化超过 3 秒、健康检查超过 2 秒 | 终止插件进程组并进入重启退避 |
| 命令等待/执行超过 5 秒、进程退出、协议损坏或输出超限 | 返回命令失败，终止插件进程组；失败命令不自动重放 |
| 一分钟内前三次运行故障 | 分别等待 0.5 秒、2 秒、5 秒后重启 |
| 一分钟内第四次运行故障 | Quarantined，停止自动重试；用户显式重启清空熔断计数 |
| 插件返回普通业务错误或缺少编辑权限 | 命令失败，插件保持可用 |
| 待执行队列达到 16 条 | 立即拒绝额外命令，不阻塞编辑器 |
| 单个插件异常 | 其他插件和基础编辑继续工作 |
| 插件包变更、禁用、移除或编辑器退出 | 停止旧进程，插件包变更需重新审核；清理临时 HOME |

处理器应是有限时、可取消的操作；不得创建依赖宿主自动重放的副作用。需要更长运行时间的工作应由插件提交到外部任务系统并快速返回任务信息，后续通过独立命令查询。为依赖缺失、空选区、非法输入等可恢复情况返回业务错误。

## 后续开发与发布门槛

每个新增插件必须提供清单、用途及依赖说明、权限理由、命令及输入输出说明、错误行为、测试和变更记录。宿主功能应通过版本化协议扩展，不得直接引用 `internal/ui`、绕过编辑预览或修改 SunBook 的配置和内部状态。

发布前要求：

1. 用 JSON Schema 或宿主验证清单；运行 `sunbook --check-plugin /absolute/path/to/<id>`，通过初始化和健康检查。此命令会执行插件本地代码，但不执行贡献的业务命令。
2. 编写命令单元测试，覆盖空输入、中文、组合字符、Emoji、反向/多个选区；保证 UTF-8 字节位置和完整字素边界。
3. 覆盖运行时依赖缺失、错误返回、异常退出、超时和错误 stdout；验证正常命令错误后仍能响应 ping。macOS 还需执行真实文件读写、默认禁网/显式联网、本机 socket 和子进程继承限制验证，不能仅检查策略字符串。
4. 在 SunBook 内验证安装、权限审核、命令面板、修改预览、一次撤销、文档变更后的过期拒绝、禁用及重启。
5. 运行宿主 `make check`、`make smoke`。CI 在 macOS/Linux 执行竞态、vet、SDK 测试、真实 PTY 分支/插件操作及失败插件隔离验证。
6. 兼容的实现修正递增补丁版本，新增命令递增次版本；不兼容协议需新增宿主 API 版本并保留旧版本支持或提供迁移说明。不得悄悄改变已发布命令的字节位置或响应含义。

宿主核心改动应补充 `internal/plugin` 的故障测试；`go test -race ./internal/plugin -run 'TestMacSandbox|TestPluginEnvironment|TestCodeChanges' -v` 在 macOS 执行沙箱、环境变量、临时目录清理与代码变更重新审核回归。新增语言 SDK 应通过相同握手、健康和编辑用例，并提供可独立安装的示例。当前流程不宣称第三方生态兼容，也不自动执行项目目录里的插件。
