# BaiduNetdiskRunner

百度网盘 Windows 客户端的界面清爽工具：隐藏广告、推广弹窗和会员推广入口。
个人自用、技术研究，不修改账号、下载或会员逻辑。

> **免责声明**：本项目为个人自用和技术研究，非官方、与百度公司无任何关联；
> 仅隐藏界面中的推广元素，不修改 `core.asar` / `app.asar`，不解锁会员、不绕过限速，
> 也不包含任何百度客户端文件。请使用自己合法安装的客户端，勿用于商业用途或再分发。
> 相关商标归百度所有；如权利人提出异议，我会立即停止维护并删除相关内容。

- 主程序约 3.5 MB；规则放在可编辑的 `BaiduNetdiskRunner.js` 中
- 两个文件和 `BaiduNetdisk.exe` 同目录，双击 exe 即用
- 内置 `F12` / `Ctrl+Shift+I`，可直接打开 DevTools 检查页面
- 网页里的“使用客户端下载”也会重定向到 Runner，不再拉起未注入的原版客户端
- 原 `core.asar` 不改，启动时重定向到生成的 `core.asar.patched`
- exe 带客户端图标和版本信息

## 用法

从仓库的 `dist\` 目录下载 `BaiduNetdiskRunner.exe` 和 `BaiduNetdiskRunner.js`，
放到客户端目录（与 `BaiduNetdisk.exe` 同一层）：

```
BaiduNetdisk\
├─ BaiduNetdisk.exe
├─ BaiduNetdiskRunner.exe
├─ BaiduNetdiskRunner.js
├─ Module\
└─ ...
```

双击 exe 即可。以后每次启动都用它；直接双击 `BaiduNetdisk.exe` 不会去广告。
首次使用、升级或修改规则后，请先完全退出百度网盘，再重新启动。

`dist\` 里只有这两个发布文件，源码、DLL 和构建中间文件不需要下载。

| 参数 | 作用 |
| --- | --- |
| `--status` | 查看状态和最近日志 |
| `--install` | 立即给当前 `YunDetectService` 注入网页下载重定向，不启动客户端 |
| `--restore` | 删除注入文件、停止 `YunDetectService`，恢复原版 |
| `--client <目录>` | 手动指定客户端目录 |
| `--no-kill` | 不结束已运行的客户端 |
| `F12` | 在客户端窗口切换 DevTools（也可用 `Ctrl+Shift+I`） |

### 网页下载重定向

网页点击“使用客户端下载”时，实际链路是：

```text
Edge -> YunDetectService(127.0.0.1:10000) -> BaiduNetdisk.exe -webdownload ...
```

Runner 启动时会自动把 `gate.dll` 注入 `YunDetectService`，把对
`BaiduNetdisk.exe` 的启动重定向到自己，并原样转发 `-webdownload` 等参数。
客户端已在运行时，Runner 只把参数转发给现有客户端，不会重复启动一份。

只想先启用网页重定向、不马上启动客户端时，可执行：

```powershell
BaiduNetdiskRunner.exe --install
```

如果重启电脑后 `YunDetectService` 被系统重新启动，先启动一次 Runner 即可再次注入。
`--restore` 会停止 `YunDetectService` 并清理注入，原版 `BaiduNetdisk.exe` 不会改名或覆盖。

### 开发者工具（F12）

通过 `BaiduNetdiskRunner.exe` 启动客户端后，让客户端窗口保持焦点，按 `F12`
或 `Ctrl+Shift+I` 会打开一个独立的 DevTools 窗口；回到客户端窗口再按一次即可关闭。
可在 Elements 面板查看当前页面类名，在 Console 面板临时验证选择器。

DevTools 只在通过 Runner 启动时可用，直接启动原版 `BaiduNetdisk.exe` 不会出现。
DevTools 中修改的 DOM/CSS 不会被保存；要长期生效，请修改
`BaiduNetdiskRunner.js` 后重启客户端。

## 自定义规则

`BaiduNetdiskRunner.js` 是明文规则文件，CSS 已合并到文件顶部的 `CSS` 常量中，
后面的 `SELECTORS` 和 `clean()` 负责处理动态元素和搜索框文案。
修改后保存，完全退出并重启客户端即可生效，不需要重新编译 exe。

规则加载优先级：

1. exe 同目录的 `BaiduNetdiskRunner.js`；
2. `%LOCALAPPDATA%\BaiduNetdiskClean\BaiduNetdiskRunner.js`；
3. 编译进 exe 的内嵌默认规则。

因此发布时可以只带 exe，或把两个文件一起提供。只改规则文件不需要重新编译；
JS 执行出错会写入 `adblock.log`。页面环境不是 Node，规则里不要使用 `require`。

### 默认头部规则

搜索栏右侧只保留最后一个直接子项（当前的 More 菜单），游戏、积分、年卡促销、
会员中心等其他头部项全部隐藏。规则使用结构选择器，不按文案匹配：

```css
.wp-disk-header.personal-pc .wp-disk-header__right.clearfix
  > .wp-disk-header__right-item:not(:last-child) {
  display: none !important;
}
```

如果以后官方调整 More 菜单的位置，再按 DevTools 中的实际 DOM 修改这条规则即可。

## 原理

1. 以挂起方式启动外层 `BaiduNetdisk.exe`，注入 `gate.dll`；
2. 外层创建 `BaiduNetdiskUnite.exe` 时把它挂住；
3. 注入 `hook.dll`，只把主进程对 `core.asar` 的读取重定向到 `core.asar.patched`；
4. `gate.dll` 同时注入 `YunDetectService`，把网页下载的客户端启动转到 Runner；
5. 恢复运行，`main.js` 里的 loader 读取并注入 `BaiduNetdiskRunner.js`。

原生校验读到的仍是原 `core.asar`，哈希不变；`app.asar` 从不修改。

## 日志

`%LOCALAPPDATA%\BaiduNetdiskClean\`：

- `main.log` —— 总流程、注入耗时
- `gate-events.log` / `hook-events.log` —— 门控和重定向
- `adblock.log` —— loader 加载来源、规则执行错误、F12/DevTools 状态

## 构建

需要 Go 和 MinGW-w64（gcc、windres）。在仓库根目录执行：

```powershell
powershell -File build.ps1
```

构建产物输出到 `dist\BaiduNetdiskRunner.exe` 和 `dist\BaiduNetdiskRunner.js`，
这两个文件会随仓库发布，方便直接下载。
只改 Go 代码时可用 `powershell -File build.ps1 -SkipNative` 跳过 DLL 重编译。

脚本会先生成 `gate.dll` / `hook.dll`（嵌入到主程序），再生成图标资源
`runner.syso`，编译主程序，最后把 `BaiduNetdiskRunner.js` 复制到 `dist`。

## 目录

```
dist/            发布文件（exe + js，可直接下载）
src/
├─ main.go  asar.go  loader.go  inject.go  client.go  win.go
├─ assets/      BaiduNetdiskRunner.js、图标、DLL
├─ native/      gate.c  hook.c  build.ps1  minhook/
└─ runner.rc    图标和 VERSIONINFO
```

## 说明

- 个人自用、技术研究，非官方项目，请勿用于商业用途或再分发。
- 只隐藏界面推广元素，不解锁 VIP、不绕过限速或付费功能。
- 不修改 `app.asar`，不修改原 `core.asar`，不拦截网络请求、不上传数据。
- 客户端更新后重新运行一次即可。
- 注入类工具可能被安全软件误报；使用风险自负。
