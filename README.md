# BaiduNetdiskRunner

百度网盘 Windows 客户端的界面清爽工具：隐藏广告、推广弹窗和会员推广入口。
个人自用、技术研究，不修改账号、下载或会员逻辑。

> **免责声明**：本项目为个人自用和技术研究，非官方、与百度公司无任何关联；
> 仅隐藏界面中的推广元素，不修改 `core.asar` / `app.asar`，不解锁会员、不绕过限速，
> 也不包含任何百度客户端文件。请使用自己合法安装的客户端，勿用于商业用途或再分发。
> 相关商标归百度所有；如权利人提出异议，我会立即停止维护并删除相关内容。

- 单文件，约 3.4 MB；不需要 Node、Python、Frida 或管理员权限
- 和 `BaiduNetdisk.exe` 同目录，双击即用
- 原 `core.asar` 不改，启动时重定向到生成的 `core.asar.patched`
- exe 带客户端图标和版本信息

## 用法

把 `BaiduNetdiskRunner.exe` 放到客户端目录：

```
BaiduNetdisk\
├─ BaiduNetdisk.exe
├─ BaiduNetdiskRunner.exe
├─ Module\
└─ ...
```

双击即可。以后每次启动都用它；直接双击 `BaiduNetdisk.exe` 不会去广告。

| 参数 | 作用 |
| --- | --- |
| `--status` | 查看状态和最近日志 |
| `--restore` | 删除注入文件，恢复原版 |
| `--client <目录>` | 手动指定客户端目录 |
| `--no-kill` | 不结束已运行的客户端 |

## 原理

1. 以挂起方式启动外层 `BaiduNetdisk.exe`，注入 `gate.dll`；
2. 外层创建 `BaiduNetdiskUnite.exe` 时把它挂住；
3. 注入 `hook.dll`，只把主进程对 `core.asar` 的读取重定向到 `core.asar.patched`；
4. 恢复运行，`main.js` 里的 loader 负责注入 CSS/JS。

原生校验读到的仍是原 `core.asar`，哈希不变；`app.asar` 从不修改。

## 日志

`%LOCALAPPDATA%\BaiduNetdiskClean\`：

- `main.log` —— 总流程、注入耗时
- `gate-events.log` / `hook-events.log` —— 门控和重定向
- `adblock.log` —— 客户端内 loader

## 构建

需要 Go 和 MinGW-w64（gcc、windres）。DLL 和图标资源都在源码里：

```powershell
# 1. 原生 DLL（gate.dll / hook.dll）
powershell -File src\native\build.ps1

# 2. 图标资源（runner.syso）
cd src
windres -O coff -o runner.syso runner.rc

# 3. 主程序
go build -trimpath -ldflags "-s -w -H windowsgui" -o ..\BaiduNetdiskRunner.exe .
```

## 目录

```
src/
├─ main.go  asar.go  loader.go  inject.go  win.go
├─ assets/      规则、图标、DLL
├─ native/      gate.c  hook.c  build.ps1  minhook/
└─ runner.rc    图标和 VERSIONINFO
```

## 说明

- 个人自用、技术研究，非官方项目，请勿用于商业用途或再分发。
- 只隐藏界面推广元素，不解锁 VIP、不绕过限速或付费功能。
- 不修改 `app.asar`，不修改原 `core.asar`，不拦截网络请求、不上传数据。
- 客户端更新后重新运行一次即可。
- 注入类工具可能被安全软件误报；使用风险自负。