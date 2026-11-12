package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type runner struct {
	clientDir  string
	runtimeDir string
	gateDLL    string
	hookDLL    string
	gateFile   string
	log        *logger
}

const (
	procCreateThread = 0x0002
	procQueryInfo    = 0x0400
	procVMOperation  = 0x0008
	procVMWrite      = 0x0020
	procVMRead       = 0x0010
	memCommitReserve = 0x00003000
	memRelease       = 0x00008000
	pageReadWrite    = 0x04
	createSuspended  = 0x00000004
	waitTimeout      = 0x00000102
)

var (
	pVirtualAllocEx     = k32.NewProc("VirtualAllocEx")
	pVirtualFreeEx      = k32.NewProc("VirtualFreeEx")
	pWriteProcessMemory = k32.NewProc("WriteProcessMemory")
	pCreateRemoteThread = k32.NewProc("CreateRemoteThread")
	pWaitSingle         = k32.NewProc("WaitForSingleObject")
	pGetExitCodeThread  = k32.NewProc("GetExitCodeThread")
	pLoadLibraryW       = k32.NewProc("LoadLibraryW")
)

func writeFileAtomic(dst string, b []byte) error {
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		// 目标可能被仍在退出的旧进程占用，退化为直接覆盖
		if err2 := os.WriteFile(dst, b, 0644); err2 != nil {
			os.Remove(tmp)
			return err2
		}
		os.Remove(tmp)
	}
	return nil
}

func prepareRuntime(clientDir, rt string, log *logger) (*runner, error) {
	// 每次运行用独立文件名，避免上一轮进程占用 DLL 导致覆盖失败
	tag := fmt.Sprintf("%d", os.Getpid())
	gate := filepath.Join(rt, "gate-"+tag+".dll")
	hook := filepath.Join(rt, "hook-"+tag+".dll")
	if err := writeFileAtomic(gate, mustAsset("gate.dll")); err != nil {
		return nil, fmt.Errorf("写入 gate.dll 失败: %w", err)
	}
	if err := writeFileAtomic(hook, mustAsset("hook.dll")); err != nil {
		return nil, fmt.Errorf("写入 hook.dll 失败: %w", err)
	}
	// 清理上一轮的 DLL（旧进程已结束，删不掉就留给下次）
	for _, pat := range []string{"gate-*.dll", "hook-*.dll"} {
		if olds, err := filepath.Glob(filepath.Join(rt, pat)); err == nil {
			for _, p := range olds {
				if p != gate && p != hook {
					os.Remove(p)
				}
			}
		}
	}

	return &runner{
		clientDir:  clientDir,
		runtimeDir: rt,
		gateDLL:    gate,
		hookDLL:    hook,
		gateFile:   filepath.Join(rt, "held.pid"),
		log:        log,
	}, nil
}

func lastLines(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:], "\n")
}

func spawnSuspended(exe, dir string, args []string) (syscall.ProcessInformation, error) {
	var pi syscall.ProcessInformation
	app, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return pi, err
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(exe))
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	cmd := syscall.StringToUTF16(strings.Join(parts, " "))
	cwd, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return pi, err
	}
	si := syscall.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(si))
	err = syscall.CreateProcess(app, &cmd[0], nil, nil, false,
		createSuspended, nil, cwd, &si, &pi)
	return pi, err
}

func injectDLL(pid uint32, dll string, timeout time.Duration) error {
	h, _, e := pOpenProc.Call(procCreateThread|procQueryInfo|procVMOperation|
		procVMWrite|procVMRead, 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess(%d) 失败: %v", pid, e)
	}
	defer pClose.Call(h)

	wide := syscall.StringToUTF16(dll)
	size := uintptr(len(wide) * 2)
	addr, _, e := pVirtualAllocEx.Call(h, 0, size, memCommitReserve, pageReadWrite)
	if addr == 0 {
		return fmt.Errorf("VirtualAllocEx 失败: %v", e)
	}
	defer pVirtualFreeEx.Call(h, addr, 0, memRelease)

	var written uintptr
	ok, _, e := pWriteProcessMemory.Call(h, addr,
		uintptr(unsafe.Pointer(&wide[0])), size,
		uintptr(unsafe.Pointer(&written)))
	if ok == 0 {
		return fmt.Errorf("WriteProcessMemory 失败: %v", e)
	}

	var tid uint32
	th, _, e := pCreateRemoteThread.Call(h, 0, 0, pLoadLibraryW.Addr(),
		addr, 0, uintptr(unsafe.Pointer(&tid)))
	if th == 0 {
		return fmt.Errorf("CreateRemoteThread 失败: %v", e)
	}
	defer pClose.Call(th)

	r, _, _ := pWaitSingle.Call(th, uintptr(timeout/time.Millisecond))
	if r == waitTimeout {
		return fmt.Errorf("等待 %s 加载超时", filepath.Base(dll))
	}

	var exit uint32
	pGetExitCodeThread.Call(th, uintptr(unsafe.Pointer(&exit)))
	if exit == 0 {
		return fmt.Errorf("%s 加载失败（LoadLibraryW 返回 0）", filepath.Base(dll))
	}
	return nil
}

func waitHeldFile(path string, timeout time.Duration) (uint32, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			s := strings.TrimSpace(string(b))
			if s != "" {
				if pid, err := strconv.Atoi(s); err == nil && pid > 0 {
					return uint32(pid), nil
				}
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	return 0, errors.New("没有检测到被挂起的 BaiduNetdiskUnite 主进程")
}

func (r *runner) terminate(pi syscall.ProcessInformation) {
	if pi.Process != 0 {
		pTermProc.Call(uintptr(pi.Process), 1)
	}
}

func (r *runner) start(args []string) error {
	adLog := filepath.Join(r.runtimeDir, "adblock.log")
	clientPid := filepath.Join(r.runtimeDir, "client.pid")
	os.Remove(adLog)
	os.Remove(clientPid)
	os.Remove(filepath.Join(r.runtimeDir, "gate-events.log"))
	os.Remove(filepath.Join(r.runtimeDir, "hook-events.log"))
	os.Remove(r.gateFile)

	if len(uniteSet()) != 0 || len(runningClientProcesses()) != 0 {
		return errors.New("仍有客户端进程在运行")
	}

	os.Setenv("NDC_DIR", r.runtimeDir)
	os.Setenv("NDC_GATE_FILE", r.gateFile)
	if exe, err := os.Executable(); err == nil {
		os.Setenv("NDC_JS", filepath.Join(filepath.Dir(exe), "BaiduNetdiskRunner.js"))
	}

	outer := filepath.Join(r.clientDir, "BaiduNetdisk.exe")
	if _, err := os.Stat(outer); err != nil {
		return fmt.Errorf("找不到原版客户端入口 %s: %w", filepath.Base(outer), err)
	}
	r.log.Printf("以挂起方式创建外层客户端: %s args=%d", filepath.Base(outer), len(args))
	pi, err := spawnSuspended(outer, r.clientDir, args)
	if err != nil {
		return fmt.Errorf("创建外层进程失败: %w", err)
	}
	defer pClose.Call(uintptr(pi.Process))
	defer pClose.Call(uintptr(pi.Thread))

	r.log.Printf("注入 gate.dll 到外层 pid=%d ...", pi.ProcessId)
	t0 := time.Now()
	if err := injectDLL(pi.ProcessId, r.gateDLL, 15*time.Second); err != nil {
		r.terminate(pi)
		return fmt.Errorf("外层门控注入失败: %v\n%s", err,
			lastLines(filepath.Join(r.runtimeDir, "gate-events.log"), 20))
	}
	r.log.Printf("gate 注入耗时 %v", time.Since(t0).Round(time.Millisecond))

	r.log.Printf("恢复外层主线程 ...")
	if prev, _, e := pResumeThr.Call(uintptr(pi.Thread)); prev == 0xFFFFFFFF {
		r.terminate(pi)
		return fmt.Errorf("ResumeThread 失败: %v", e)
	}

	r.log.Printf("等待被挂起的主进程 ...")
	pid, err := waitHeldFile(r.gateFile, 25*time.Second)
	if err != nil {
		r.terminate(pi)
		return fmt.Errorf("%v\n%s", err,
			lastLines(filepath.Join(r.runtimeDir, "gate-events.log"), 30))
	}
	susp := suspendedCount(uint32(pid))
	r.log.Printf("捕获主进程 pid=%d threads=%d suspended=%d",
		pid, len(threadIDs(uint32(pid))), susp)
	if susp == 0 {
		r.log.Printf("警告: 主进程未被挂起，注入可能已晚")
	}

	r.log.Printf("注入 hook.dll 到 pid=%d ...", pid)
	t1 := time.Now()
	if err := injectDLL(pid, r.hookDLL, 15*time.Second); err != nil {
		r.terminate(pi)
		return fmt.Errorf("文件重定向注入失败: %v\n%s", err,
			lastLines(filepath.Join(r.runtimeDir, "hook-events.log"), 20))
	}
	r.log.Printf("hook 注入耗时 %v", time.Since(t1).Round(time.Millisecond))
	r.log.Printf("hook 注入完成，挂起数=%d", suspendedCount(uint32(pid)))

	for round := 0; round < 4; round++ {
		n := resumeAll(uint32(pid))
		r.log.Printf("第 %d 轮恢复 %d 个挂起计数，当前挂起数=%d",
			round+1, n, suspendedCount(uint32(pid)))
		if suspendedCount(uint32(pid)) == 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(adLog); err == nil &&
			strings.Contains(string(b), "loader ready") {
			os.WriteFile(clientPid, []byte(strconv.Itoa(int(pi.ProcessId))), 0644)
			r.log.Printf("客户端已加载去广告规则: %s",
				strings.TrimSpace(string(b)))
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("客户端已启动，但 20 秒内没有看到 loader ready；请查看 " + adLog)
}

func (r *runner) installYDGate(log *logger) {
	pids := runningProcesses("YunDetectService.exe")
	if len(pids) == 0 {
		exe := filepath.Join(r.clientDir, "YunDetectService.exe")
		if _, err := os.Stat(exe); err == nil {
			cmd := exec.Command(exe)
			cmd.Dir = r.clientDir
			if err := cmd.Start(); err != nil {
				log.Printf("启动 YunDetectService 失败: %v", err)
				return
			}
			log.Printf("已启动 YunDetectService，等待注入 ...")
			for i := 0; i < 30 && len(pids) == 0; i++ {
				time.Sleep(100 * time.Millisecond)
				pids = runningProcesses("YunDetectService.exe")
			}
		}
	}
	if len(pids) == 0 {
		log.Printf("YunDetectService 未运行，跳过网页下载重定向")
		return
	}
	for pid := range pids {
		if pid == uint32(os.Getpid()) {
			continue
		}
		if err := injectDLL(pid, r.gateDLL, 10*time.Second); err != nil {
			log.Printf("网页下载重定向注入 pid=%d 失败: %v", pid, err)
		} else {
			log.Printf("网页下载重定向已注入 pid=%d", pid)
		}
	}
}

func (r *runner) waitStopped(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(runningClientProcesses()) == 0 {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
