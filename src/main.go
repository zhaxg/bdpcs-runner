package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type logger struct {
	f *os.File
}

func newLogger(path string) *logger {
	f, _ := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	return &logger{f: f}
}

func (l *logger) Printf(format string, a ...any) {
	if l == nil || l.f == nil {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05") + " " + fmt.Sprintf(format, a...) + "\n"
	l.f.WriteString(line)
}

func (l *logger) Close() {
	if l != nil && l.f != nil {
		l.f.Close()
	}
}

type config struct {
	client string
	status bool
	restore bool
	noKill bool
}

func parseFlags() config {
	var c config
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--status":
			c.status = true
		case a == "--restore":
			c.restore = true
		case a == "--no-kill":
			c.noKill = true
		case a == "--client":
			if i+1 < len(args) {
				i++
				c.client = args[i]
			}
		case strings.HasPrefix(a, "--client="):
			c.client = strings.TrimPrefix(a, "--client=")
		}
	}
	return c
}

func validClient(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "BaiduNetdisk.exe")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "Module", "BrowserEngine", "resources", "core.asar"))
	return err == nil
}

func resolveClient(flag string) string {
	if flag != "" {
		return filepath.Clean(flag)
	}
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	// 首选 exe 自己所在目录（BaiduNetdiskRunner.exe 与 BaiduNetdisk.exe 同目录）
	if validClient(exeDir) {
		return exeDir
	}
	cand := filepath.Clean(filepath.Join(exeDir, "..", "BaiduNetdisk"))
	if validClient(cand) {
		return cand
	}
	return exeDir
}

func runtimeDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	d := filepath.Join(base, "BaiduNetdiskClean")
	os.MkdirAll(d, 0755)
	return d
}

var (
	pCreateMutex = k32.NewProc("CreateMutexW")
	mutexHandle  uintptr
)

func singleInstance() bool {
	name, _ := syscall.UTF16PtrFromString(`Local\BaiduNetdiskClean_Mutex`)
	h, _, err := pCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(name)))
	mutexHandle = h
	if e, ok := err.(syscall.Errno); ok && e == 183 {
		return false
	}
	return true
}

type patchedMeta struct {
	SourceHash  string `json:"sourceSha256"`
	SourceSize  int64  `json:"sourceSize"`
	RulesHash   string `json:"rulesHash"`
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	GeneratedAt string `json:"generatedAt"`
}

func ensurePatched(client string, log *logger) (string, string, error) {
	dir := filepath.Join(client, "Module", "BrowserEngine", "resources")
	asar := filepath.Join(dir, "core.asar")
	patched := asar + ".patched"
	metaPath := patched + ".meta"

	css := mustAsset("adblock.css")
	js := mustAsset("adblock.js")
	rh := rulesHash(css, js)

	src, err := os.ReadFile(asar)
	if err != nil {
		return "", "", fmt.Errorf("读取 core.asar 失败: %w", err)
	}
	srcHash := sha256hex(src)
	meta := patchedMeta{
		SourceHash: srcHash,
		SourceSize: int64(len(src)),
		RulesHash:  rh,
		Tool:       toolVersion,
	}

	if b, err := os.ReadFile(metaPath); err == nil {
		var old patchedMeta
		if json.Unmarshal(b, &old) == nil &&
			old.SourceHash == meta.SourceHash && old.SourceSize == meta.SourceSize &&
			old.RulesHash == meta.RulesHash && old.Tool == meta.Tool {
			if pb, err := os.ReadFile(patched); err == nil {
				if err := verifyPatched(pb); err == nil {
					log.Printf("复用已生成的 .patched (%s, %s)", old.Version, describeRules(css, js))
					return patched, old.Version, nil
				}
				log.Printf("现有 .patched 校验失败，重新生成: %v", err)
			}
		}
	}

	log.Printf("生成 core.asar.patched ... (%s)", describeRules(css, js))
	mainOld, err := readMainFromAsar(src)
	if err != nil {
		return "", "", fmt.Errorf("读取 main.js 失败: %w", err)
	}
	if strings.Contains(string(mainOld), startMarker) {
		return "", "", fmt.Errorf("原 core.asar 里已经存在注入标记，请先还原")
	}
	loader := buildLoader(css, js)
	mainNew := insertLoader(mainOld, loader)
	out, version, orig, err := buildPatched(src, mainNew)
	if err != nil {
		return "", "", fmt.Errorf("重建 asar 失败: %w", err)
	}
	if err := selfCheck(out, orig); err != nil {
		return "", "", err
	}

	tmp := patched + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return "", "", fmt.Errorf("写入 .patched 失败: %w", err)
	}
	if err := os.Rename(tmp, patched); err != nil {
		os.Remove(tmp)
		return "", "", fmt.Errorf("替换 .patched 失败: %w", err)
	}
	meta.Version = version
	meta.GeneratedAt = time.Now().Format("2006-01-02T15:04:05")
	mb, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(metaPath, mb, 0644)
	log.Printf("已生成 .patched: version=%s size=%d sha256=%s", version, len(out), sha256hex(out))
	return patched, version, nil
}

func waitStopped(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(runningProcesses("BaiduNetdisk.exe", "BaiduNetdiskUnite.exe", "baidunetdiskhost.exe")) == 0 {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func doStatus(client, rt string, log *logger) {
	asar := filepath.Join(client, "Module", "BrowserEngine", "resources", "core.asar")
	patched := asar + ".patched"
	var sb strings.Builder
	fmt.Fprintf(&sb, "客户端目录: %s\n", client)
	fmt.Fprintf(&sb, "运行状态: %v\n\n", runningProcesses("BaiduNetdisk.exe", "BaiduNetdiskUnite.exe", "baidunetdiskhost.exe"))

	if src, err := os.ReadFile(asar); err != nil {
		fmt.Fprintf(&sb, "core.asar: 读取失败 %v\n", err)
	} else {
		version := "?"
		if root, data, err := parseAsar(src); err == nil {
			version = versionOf(root, data)
		}
		fmt.Fprintf(&sb, "core.asar: %d B, version %s\n", len(src), version)
		fmt.Fprintf(&sb, "sha256: %s\n", sha256hex(src))
	}
	if pb, err := os.ReadFile(patched); err != nil {
		fmt.Fprintf(&sb, ".patched: 不存在\n")
	} else if err := verifyPatched(pb); err != nil {
		fmt.Fprintf(&sb, ".patched: 异常 (%v), %d B\n", err, len(pb))
	} else {
		fmt.Fprintf(&sb, ".patched: 正常, %d B\n", len(pb))
	}
	css := mustAsset("adblock.css")
	js := mustAsset("adblock.js")
	fmt.Fprintf(&sb, "内置规则: %s\n", describeRules(css, js))
	fmt.Fprintf(&sb, "注入方式: 原生 DLL（MinHook，无 Frida）\n")
	fmt.Fprintf(&sb, "日志目录: %s\n\n", rt)
	fmt.Fprintf(&sb, "--- main.log ---\n%s\n", lastLines(filepath.Join(rt, "main.log"), 30))
	text := sb.String()
	if len(text) > 6000 {
		text = text[len(text)-6000:]
	}
	messageBox("百度网盘清爽版 - 状态", text, false)
}

func doRestore(client, rt string, cfg config, log *logger) {
	if !cfg.noKill {
		n := killClient()
		log.Printf("restore: 结束 %d 个进程", n)
		waitStopped(6 * time.Second)
	}
	res := filepath.Join(client, "Module", "BrowserEngine", "resources")
	patched := filepath.Join(res, "core.asar.patched")
	removed := 0
	for _, p := range []string{patched, patched + ".meta", patched + ".tmp"} {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	os.RemoveAll(filepath.Join(client, "Module", "BrowserEngine", "adblock"))
	log.Printf("restore: 删除 %d 个文件", removed)
	messageBox("百度网盘清爽版 - 还原",
		"已停止客户端并删除注入文件。\n原 core.asar 没有被修改，直接启动客户端即可。", false)
}

func fail(log *logger, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Printf("ERROR " + msg)
	messageBox("百度网盘清爽版 - 错误", msg+"\n\n日志: "+filepath.Join(runtimeDir(), "main.log"), true)
	os.Exit(1)
}

func main() {
	cfg := parseFlags()
	client := resolveClient(cfg.client)
	rt := runtimeDir()
	log := newLogger(filepath.Join(rt, "main.log"))
	defer log.Close()
	log.Printf("=== BaiduNetdiskRunner %s ===", toolVersion)
	log.Printf("client=%s runtime=%s", client, rt)

	if cfg.status {
		doStatus(client, rt, log)
		return
	}
	if cfg.restore {
		if !validClient(client) {
			fail(log, "找不到客户端: %s", client)
		}
		doRestore(client, rt, cfg, log)
		return
	}

	if !validClient(client) {
		fail(log, "找不到客户端: %s\n请把 BaiduNetdiskRunner.exe 放到 BaiduNetdisk.exe 同目录，或用 --client 指定", client)
	}
	if !singleInstance() {
		fail(log, "已有一个 NetdiskClean 正在运行")
	}

	if !cfg.noKill {
		if n := killClient(); n > 0 {
			log.Printf("已结束 %d 个客户端进程", n)
		}
		if !waitStopped(8 * time.Second) {
			fail(log, "客户端进程未能完全退出，请手动退出后重试")
		}
	} else if len(runningProcesses("BaiduNetdisk.exe", "BaiduNetdiskUnite.exe")) > 0 {
		fail(log, "客户端正在运行（--no-kill）。请先退出客户端")
	}

	_, version, err := ensurePatched(client, log)
	if err != nil {
		fail(log, "%v", err)
	}
	log.Printf("patched 准备完成 (version %s)", version)

	r, err := prepareRuntime(client, rt, log)
	if err != nil {
		fail(log, "%v", err)
	}
	if err := r.start(); err != nil {
		fail(log, "%v", err)
	}
	log.Printf("完成")
}