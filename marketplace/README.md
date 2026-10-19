# SunBook Plugin Store

GitHub 托管仓库：https://github.com/Sunmedalia/SunBook-plugins

目录：`https://raw.githubusercontent.com/Sunmedalia/SunBook-plugins/main/index.json`

此目录保留发布目录与安装包的可审核副本。插件源代码位于主仓库 `examples/`，每个 ZIP 包含同名插件目录和完整源代码、README、SDK，无需克隆仓库或执行安装脚本。

运行 `python3 scripts/package_plugins.py` 生成固定时间戳的 ZIP 和 SHA-256 目录。修改插件时先递增版本，不覆盖已发布版本的 ZIP。将新 ZIP 与 index.json 一起提交到托管仓库；旧包继续保留，以支持读取旧目录的客户端。CI 检查包内容、版本、清单与校验值。

目录采用 `schemaVersion: 1`，每个条目包含 id、name、version、description、author、repository、runtime、url、sha256。目录和包下载仅接受 HTTPS GitHub 地址；不提供签名发行方验证，目录维护者是信任来源。下载不会执行代码，安装后用户需另行审核权限并启用。

商店不自动更新或覆盖现有插件。需要更换版本时先禁用、移除，再重新下载。网络不可用时可继续使用本地插件或从本地目录安装。
