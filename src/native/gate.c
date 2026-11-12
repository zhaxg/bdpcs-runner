/*
 * gate.dll has two modes:
 * - injected into outer BaiduNetdisk.exe: pause BaiduNetdiskUnite.exe and
 *   publish its PID through %NDC_GATE_FILE% for hook.dll injection;
 * - injected into YunDetectService.exe: redirect CreateProcessW for
 *   BaiduNetdisk.exe to BaiduNetdiskRunner.exe with the same arguments.
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
static WCHAR g_runner[MAX_PATH * 2];
static int g_yd_mode;
static HANDLE g_gate_mutex;

static void read_env(void)
{
    DWORD n;
    n = GetEnvironmentVariableW(L"NDC_DIR", g_dir, MAX_PATH * 2);
    if (n == 0 || n >= MAX_PATH * 2) g_dir[0] = 0;
    n = GetEnvironmentVariableW(L"NDC_GATE_FILE", g_gate_file, MAX_PATH * 2);
    if (n == 0 || n >= MAX_PATH * 2) g_gate_file[0] = 0;
    if (!g_dir[0]) {
        n = GetEnvironmentVariableW(L"LOCALAPPDATA", g_dir, MAX_PATH * 2);
        if (n > 0 && n < MAX_PATH * 2) {
            wcscat(g_dir, L"\\BaiduNetdiskClean");
        } else {
            g_dir[0] = 0;
        }
    }
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

static const WCHAR *base_name(const WCHAR *path)
{
    const WCHAR *base = path;
    const WCHAR *p;
    if (!path) return L"";
    for (p = path; *p; ++p) {
        if (*p == L'\\' || *p == L'/') base = p + 1;
    }
    return base;
}

static int equals_i(const WCHAR *a, const WCHAR *b)
{
    size_t i;
    if (!a || !b) return 0;
    for (i = 0; a[i] && b[i]; ++i) {
        if (towlower(a[i]) != towlower(b[i])) return 0;
    }
    return a[i] == 0 && b[i] == 0;
}

static int is_client_exe(const WCHAR *path)
{
    return equals_i(base_name(path), L"BaiduNetdisk.exe");
}

static int command_is_client(const WCHAR *cmd)
{
    WCHAR first[MAX_PATH * 2];
    const WCHAR *p = cmd;
    size_t i = 0;
    if (!cmd) return 0;
    while (*p == L' ' || *p == L'\t') ++p;
    if (*p == L'"') {
        ++p;
        while (*p && *p != L'"' && i + 1 < MAX_PATH * 2) first[i++] = *p++;
    } else {
        while (*p && *p != L' ' && *p != L'\t' && i + 1 < MAX_PATH * 2) {
            first[i++] = *p++;
        }
    }
    first[i] = 0;
    return is_client_exe(first);
}

static void build_yd_command(const WCHAR *cmd, WCHAR *out, size_t out_len)
{
    const WCHAR *tail = L"";
    if (cmd && cmd[0]) {
        if (cmd[0] == L'"') {
            const WCHAR *q = wcschr(cmd + 1, L'"');
            if (q) tail = q + 1;
        } else {
            const WCHAR *sp = wcschr(cmd, L' ');
            tail = sp ? sp : cmd + wcslen(cmd);
        }
    }
    swprintf(out, out_len, L"\"%ls\"%ls", g_runner, tail);
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

    if (g_yd_mode && (is_client_exe(app) || command_is_client(cmd))) {
        WCHAR newcmd[32768];
        WCHAR msg[512];
        build_yd_command(cmd, newcmd, 32768);
        ok = real_CreateProcessW(g_runner, newcmd, pa, ta, inherit, flags,
                                 env, cwd, si, pi);
        swprintf(msg, 512,
                 L"YD REDIRECT ok=%d app=%.100ls cmd=%.140ls",
                 ok, appbuf, cmdbuf);
        log_event(msg);
        return ok;
    }

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
        WCHAR selfPath[MAX_PATH * 2];
        WCHAR *slash;
        WCHAR msg[256];

        DisableThreadLibraryCalls(self);
        selfPath[0] = 0;
        GetModuleFileNameW(NULL, selfPath, MAX_PATH * 2);
        g_yd_mode = equals_i(base_name(selfPath), L"YunDetectService.exe");
        read_env();
        swprintf(msg, 256, L"Local\\BaiduNetdiskClean_Gate_%ls",
                 g_yd_mode ? L"YD" : L"Client");
        g_gate_mutex = CreateMutexW(NULL, FALSE, msg);
        if (g_gate_mutex && GetLastError() == ERROR_ALREADY_EXISTS) {
            log_event(L"gate already active, skip duplicate install");
            CloseHandle(g_gate_mutex);
            g_gate_mutex = NULL;
            return TRUE;
        }
        wcscpy(g_runner, selfPath);
        slash = wcsrchr(g_runner, L'\\');
        if (slash) {
            wcscpy(slash + 1, L"BaiduNetdiskRunner.exe");
        } else {
            wcscpy(g_runner, L"BaiduNetdiskRunner.exe");
        }

        st = MH_Initialize();
        k32 = GetModuleHandleW(L"kernel32.dll");
        target = (void *)GetProcAddress(k32, "CreateProcessW");
        if (st == MH_OK && target) {
            st = MH_CreateHook(target, (void *)HookCreateProcessW,
                               (void **)&real_CreateProcessW);
            if (st == MH_OK) st = MH_EnableHook(target);
        }
        swprintf(msg, 256, L"gate installed mode=%ls hook status=%d target=%p",
                 g_yd_mode ? L"yundetect" : L"client", st, target);
        log_event(msg);
    }
    return TRUE;
}
