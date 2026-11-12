package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	th32csSnapProc      = 0x00000002
	th32csSnapThr       = 0x00000004
	processTerminate    = 0x0001
	threadSuspendResume = 0x0002
)

var (
	k32         = syscall.NewLazyDLL("kernel32.dll")
	user32      = syscall.NewLazyDLL("user32.dll")
	pSnap       = k32.NewProc("CreateToolhelp32Snapshot")
	pProcFirst  = k32.NewProc("Process32FirstW")
	pProcNext   = k32.NewProc("Process32NextW")
	pThrFirst   = k32.NewProc("Thread32First")
	pThrNext    = k32.NewProc("Thread32Next")
	pOpenProc   = k32.NewProc("OpenProcess")
	pTermProc   = k32.NewProc("TerminateProcess")
	pOpenThr    = k32.NewProc("OpenThread")
	pResumeThr  = k32.NewProc("ResumeThread")
	pSuspendThr = k32.NewProc("SuspendThread")
	pClose      = k32.NewProc("CloseHandle")
	pMessageBox = user32.NewProc("MessageBoxW")
)

type processEntry32W struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

type threadEntry32 struct {
	Size           uint32
	CntUsage       uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

func processList() []processEntry32W {
	var out []processEntry32W
	snap, _, _ := pSnap.Call(th32csSnapProc, 0)
	if snap == uintptr(syscall.InvalidHandle) || snap == 0 {
		return out
	}
	defer pClose.Call(snap)
	var pe processEntry32W
	pe.Size = uint32(unsafe.Sizeof(pe))
	r, _, _ := pProcFirst.Call(snap, uintptr(unsafe.Pointer(&pe)))
	for r != 0 {
		out = append(out, pe)
		r, _, _ = pProcNext.Call(snap, uintptr(unsafe.Pointer(&pe)))
	}
	return out
}

func runningProcesses(names ...string) map[uint32]string {
	want := map[string]bool{}
	for _, n := range names {
		want[strings.ToLower(n)] = true
	}
	out := map[uint32]string{}
	for _, pe := range processList() {
		name := syscall.UTF16ToString(pe.ExeFile[:])
		if want[strings.ToLower(name)] {
			out[pe.ProcessID] = name
		}
	}
	return out
}

func runningClientProcesses() map[uint32]string {
	out := runningProcesses("BaiduNetdisk.exe",
		"BaiduNetdiskUnite.exe", "baidunetdiskhost.exe")
	delete(out, uint32(os.Getpid()))
	return out
}

func injectedClientRunning() bool {
	b, err := os.ReadFile(filepath.Join(runtimeDir(), "client.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false
	}
	_, ok := runningProcesses("BaiduNetdisk.exe")[uint32(pid)]
	return ok
}

func uniteSet() map[uint32]bool {
	out := map[uint32]bool{}
	for _, pe := range processList() {
		if strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), "BaiduNetdiskUnite.exe") {
			out[pe.ProcessID] = true
		}
	}
	return out
}

func killClient() int {
	targets := map[string]bool{
		"baidunetdisk.exe":      true,
		"baidunetdiskunite.exe": true,
		"baidunetdiskhost.exe":  true,
	}
	n := 0
	self := uint32(os.Getpid())
	for _, pe := range processList() {
		if pe.ProcessID == self {
			continue
		}
		name := strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))
		if !targets[name] {
			continue
		}
		h, _, _ := pOpenProc.Call(processTerminate, 0, uintptr(pe.ProcessID))
		if h != 0 {
			pTermProc.Call(h, 1)
			pClose.Call(h)
			n++
		}
	}
	return n
}

func killYunDetect() int {
	n := 0
	for _, pe := range processList() {
		if !strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), "YunDetectService.exe") {
			continue
		}
		h, _, _ := pOpenProc.Call(processTerminate, 0, uintptr(pe.ProcessID))
		if h != 0 {
			pTermProc.Call(h, 1)
			pClose.Call(h)
			n++
		}
	}
	return n
}

func threadIDs(pid uint32) []uint32 {
	var ids []uint32
	snap, _, _ := pSnap.Call(th32csSnapThr, 0)
	if snap == uintptr(syscall.InvalidHandle) || snap == 0 {
		return ids
	}
	defer pClose.Call(snap)
	var te threadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	r, _, _ := pThrFirst.Call(snap, uintptr(unsafe.Pointer(&te)))
	for r != 0 {
		if te.OwnerProcessID == pid {
			ids = append(ids, te.ThreadID)
		}
		r, _, _ = pThrNext.Call(snap, uintptr(unsafe.Pointer(&te)))
	}
	return ids
}

func suspendedCount(pid uint32) int {
	n := 0
	for _, tid := range threadIDs(pid) {
		h, _, _ := pOpenThr.Call(threadSuspendResume, 0, uintptr(tid))
		if h == 0 {
			continue
		}
		prev, _, _ := pSuspendThr.Call(h)
		if prev != 0xFFFFFFFF {
			if prev > 0 {
				n++
			}
			pResumeThr.Call(h)
		}
		pClose.Call(h)
	}
	return n
}

func resumeAll(pid uint32) int {
	resumed := 0
	for _, tid := range threadIDs(pid) {
		h, _, _ := pOpenThr.Call(threadSuspendResume, 0, uintptr(tid))
		if h == 0 {
			continue
		}
		for i := 0; i < 16; i++ {
			prev, _, _ := pResumeThr.Call(h)
			if prev == 0 || prev == 0xFFFFFFFF {
				if prev != 0 {
					resumed++
				}
				break
			}
			resumed++
		}
		pClose.Call(h)
	}
	return resumed
}

func messageBox(title, text string, isError bool) {
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(text)
	flags := uintptr(0x00010000 | 0x00000040) // SETFOREGROUND | ICONINFORMATION
	if isError {
		flags = 0x00010000 | 0x00000010 // SETFOREGROUND | ICONERROR
	}
	pMessageBox.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), flags)
}
