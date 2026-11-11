# BaiduNetdiskRunner

百度网盘 Windows 客户端的清爽启动器。去掉广告、推广弹窗和会员推广入口，
只改界面显示，不动账号、下载和会员逻辑。

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

- 非官方修改；客户端更新后重新运行一次即可。
- 不改 `app.asar`，不解锁 VIP，不绕过限速。
- 注入类工具可能被安全软件误报。