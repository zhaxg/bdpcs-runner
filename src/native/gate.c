/*
 * gate.dll —— 注入到外层 BaiduNetdisk.exe（VMProtect 加壳）。
 * 用 MinHook 挂 kernel32!CreateProcessW（和 Frida 的做法一致，不改 VMProtect
 * 保护的导入表）。外层创建 BaiduNetdiskUnite.exe 后，再 SuspendThread 4 次
 * 把子线程挂住，把 PID 写到 %NDC_GATE_FILE%，由启动器注入 hook.dll 后恢复。
 */
#include <windows.h>
#include <wchar.h>
#include <wctype.h>
#include "MinHook.h"

typedef BOOL (WINAPI *CreateProcessW_t)(LPCWSTR, LPWSTR, LPSECURITY_ATTRIBUTES,
    LPSECURITY_ATTRIBUTES, BOOL, DWORD, LPVOID, LPCWSTR, LPSTARTUPINFOW,
    LPPROCESS_INFORMATION);

static CreateProcessW_t real_CreateProcessW;
static WCHAR g_dir[MAX_PATH * 2];
static WCHAR g_gate_file[MAX_PATH * 2];

static void read_env(void)
{
    DWORD n;
    n = GetEnvironmentVariableW(L"NDC_DIR", g_dir, MAX_PATH * 2);
    if (n == 0 || n >= MAX_PATH * 2) g_dir[0] = 0;
    n = GetEnvironmentVariableW(L"NDC_GATE_FILE", g_gate_file, MAX_PATH * 2);
    if (n == 0 || n >= MAX_PATH * 2) g_gate_file[0] = 0;
}

static void append_utf8(const WCHAR *path, const WCHAR *text)
{
    char buf[4096];
    int n;
    HANDLE h;
    DWORD written = 0;

    if (!path || !path[0] || !text) return;
    n = WideCharToMultiByte(CP_UTF8, 0, text, -1, buf, sizeof(buf) - 1,
                            NULL, NULL);
    if (n <= 1) return;
    h = CreateFileW(path, FILE_APPEND_DATA,
                    FILE_SHARE_READ | FILE_SHARE_WRITE, NULL,
                    OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (h == INVALID_HANDLE_VALUE) return;
    WriteFile(h, buf, (DWORD)(n - 1), &written, NULL);
    CloseHandle(h);
}

static void log_event(const WCHAR *msg)
{
    WCHAR path[MAX_PATH * 2];
    WCHAR line[1536];
    SYSTEMTIME st;
    if (!g_dir[0]) return;
    GetLocalTime(&st);
    swprintf(path, MAX_PATH * 2, L"%ls\\gate-events.log", g_dir);
    swprintf(line, 1536, L"%04u-%02u-%02u %02u:%02u:%02u %ls\n",
             st.wYear, st.wMonth, st.wDay, st.wHour, st.wMinute, st.wSecond,
             msg);
    append_utf8(path, line);
}

static int contains_i(const WCHAR *hay, const WCHAR *needle)
{
    size_t hn, i;
    if (!hay || !needle) return 0;
    hn = wcslen(needle);
    if (!hn) return 0;
    for (; *hay; ++hay) {
        for (i = 0; i < hn; ++i) {
            if (!hay[i]) break;
            if (towlower(hay[i]) != towlower(needle[i])) break;
        }
        if (i == hn) return 1;
    }
    return 0;
}

static BOOL WINAPI HookCreateProcessW(LPCWSTR app, LPWSTR cmd,
    LPSECURITY_ATTRIBUTES pa, LPSECURITY_ATTRIBUTES ta, BOOL inherit,
    DWORD flags, LPVOID env, LPCWSTR cwd, LPSTARTUPINFOW si,
    LPPROCESS_INFORMATION pi)
{
    WCHAR appbuf[160];
    WCHAR cmdbuf[200];
    BOOL ok;

    appbuf[0] = 0;
    cmdbuf[0] = 0;
    if (app) wcsncpy(appbuf, app, 159);
    if (cmd) wcsncpy(cmdbuf, cmd, 199);
    appbuf[159] = 0;
    cmdbuf[199] = 0;

    ok = real_CreateProcessW(app, cmd, pa, ta, inherit, flags, env, cwd,
                             si, pi);

    if (ok && pi && (contains_i(app, L"baidunetdiskunite.exe") ||
                     contains_i(cmd, L"baidunetdiskunite.exe"))) {
        DWORD pid = pi->dwProcessId;
        WCHAR msg[512];
        int i;

        for (i = 0; i < 4; ++i) SuspendThread(pi->hThread);

        swprintf(msg, 512,
                 L"GATED pid=%lu flags=0x%08lX app=%.100ls cmd=%.140ls",
                 (unsigned long)pid, (unsigned long)flags, appbuf, cmdbuf);
        log_event(msg);

        if (g_gate_file[0]) {
            WCHAR line[32];
            swprintf(line, 32, L"%lu\n", (unsigned long)pid);
            append_utf8(g_gate_file, line);
        }
    } else {
        WCHAR msg[512];
        swprintf(msg, 512,
                 L"CreateProcessW ok=%d other app=%.100ls cmd=%.140ls",
                 ok, appbuf, cmdbuf);
        log_event(msg);
    }
    return ok;
}

BOOL WINAPI DllMain(HINSTANCE self, DWORD reason, LPVOID reserved)
{
    (void)reserved;
    if (reason == DLL_PROCESS_ATTACH) {
        HMODULE k32;
        void *target;
        MH_STATUS st;
        WCHAR msg[256];

        DisableThreadLibraryCalls(self);
        read_env();

        st = MH_Initialize();
        k32 = GetModuleHandleW(L"kernel32.dll");
        target = (void *)GetProcAddress(k32, "CreateProcessW");
        if (st == MH_OK && target) {
            st = MH_CreateHook(target, (void *)HookCreateProcessW,
                               (void **)&real_CreateProcessW);
            if (st == MH_OK) st = MH_EnableHook(target);
        }
        swprintf(msg, 256, L"gate installed, hook status=%d target=%p", st,
                 target);
        log_event(msg);
    }
    return TRUE;
}