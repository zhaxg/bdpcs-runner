/*
 * hook.dll —— 注入到 BaiduNetdiskUnite.exe 主进程。
 * 用 MinHook 挂 kernel32!CreateFileW/CreateFileA，通过返回地址判断调用模块：
 * 只有主程序自己打开的 core.asar 换成 core.asar.patched；
 * ucrtbase 等原生校验代码的打开不拦截，校验仍读原包。
 */
#include <windows.h>
#include <wchar.h>
#include <wctype.h>
#include <string.h>
#include "MinHook.h"

typedef HANDLE (WINAPI *CreateFileW_t)(LPCWSTR, DWORD, DWORD,
    LPSECURITY_ATTRIBUTES, DWORD, DWORD, HANDLE);
typedef HANDLE (WINAPI *CreateFileA_t)(LPCSTR, DWORD, DWORD,
    LPSECURITY_ATTRIBUTES, DWORD, DWORD, HANDLE);

static CreateFileW_t real_CreateFileW;
static CreateFileA_t real_CreateFileA;
static WCHAR g_dir[MAX_PATH * 2];
static HMODULE g_main;

static void read_env(void)
{
    DWORD n = GetEnvironmentVariableW(L"NDC_DIR", g_dir, MAX_PATH * 2);
    if (n == 0 || n >= MAX_PATH * 2) g_dir[0] = 0;
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
    WCHAR line[1024];
    SYSTEMTIME st;
    if (!g_dir[0]) return;
    GetLocalTime(&st);
    swprintf(path, MAX_PATH * 2, L"%ls\\hook-events.log", g_dir);
    swprintf(line, 1024, L"%04u-%02u-%02u %02u:%02u:%02u %ls\n",
             st.wYear, st.wMonth, st.wDay, st.wHour, st.wMinute, st.wSecond,
             msg);
    append_utf8(path, line);
}

/* 返回地址所在模块是否就是主程序（BaiduNetdiskUnite.exe） */
static int caller_is_main(void)
{
    HMODULE h = NULL;
    void *ret = __builtin_return_address(0);
    if (!GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                            GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                            (LPCWSTR)ret, &h))
        return 0;
    return h == g_main;
}

static int ends_with_i(const WCHAR *s, const WCHAR *suffix)
{
    size_t n, sn;
    if (!s || !suffix) return 0;
    n = wcslen(s);
    sn = wcslen(suffix);
    if (n < sn) return 0;
    s += n - sn;
    while (*suffix) {
        if (towlower(*s) != towlower(*suffix)) return 0;
        ++s;
        ++suffix;
    }
    return 1;
}

static int ends_with_i_a(const char *s, const char *suffix)
{
    size_t n, sn;
    if (!s || !suffix) return 0;
    n = strlen(s);
    sn = strlen(suffix);
    if (n < sn) return 0;
    s += n - sn;
    while (*suffix) {
        char a = *s, b = *suffix;
        if (a >= 'A' && a <= 'Z') a = (char)(a - 'A' + 'a');
        if (b >= 'A' && b <= 'Z') b = (char)(b - 'A' + 'a');
        if (a != b) return 0;
        ++s;
        ++suffix;
    }
    return 1;
}

/* 只关心“打开已存在的 core.asar 做读取”，写/创建语义原样放行 */
static int should_redirect(DWORD access, DWORD disposition)
{
    if (access & GENERIC_WRITE) return 0;
    if (disposition == CREATE_ALWAYS || disposition == TRUNCATE_EXISTING)
        return 0;
    return 1;
}

static HANDLE WINAPI HookCreateFileW(LPCWSTR name, DWORD access,
    DWORD share, LPSECURITY_ATTRIBUTES sa, DWORD disposition,
    DWORD flags, HANDLE tmpl)
{
    if (name && caller_is_main() && should_redirect(access, disposition) &&
        ends_with_i(name, L"core.asar")) {
        size_t len = wcslen(name);
        WCHAR buf[1024];
        if (len - 9 + 18 <= 1024) {
            wmemcpy(buf, name, len - 9);
            wcscpy(buf + (len - 9), L"core.asar.patched");
            if (GetFileAttributesW(buf) != INVALID_FILE_ATTRIBUTES) {
                HANDLE h = real_CreateFileW(buf, access, share, sa,
                                            disposition, flags, tmpl);
                if (h != INVALID_HANDLE_VALUE)
                    log_event(L"redirected core.asar -> core.asar.patched");
                return h;
            }
        }
    }
    return real_CreateFileW(name, access, share, sa, disposition, flags, tmpl);
}

static HANDLE WINAPI HookCreateFileA(LPCSTR name, DWORD access,
    DWORD share, LPSECURITY_ATTRIBUTES sa, DWORD disposition,
    DWORD flags, HANDLE tmpl)
{
    if (name && caller_is_main() && should_redirect(access, disposition) &&
        ends_with_i_a(name, "core.asar")) {
        size_t len = strlen(name);
        char buf[1024];
        if (len - 9 + 18 <= 1024) {
            memcpy(buf, name, len - 9);
            strcpy(buf + (len - 9), "core.asar.patched");
            if (GetFileAttributesA(buf) != INVALID_FILE_ATTRIBUTES) {
                HANDLE h = real_CreateFileA(buf, access, share, sa,
                                            disposition, flags, tmpl);
                if (h != INVALID_HANDLE_VALUE)
                    log_event(L"redirected core.asar (A) -> core.asar.patched");
                return h;
            }
        }
    }
    return real_CreateFileA(name, access, share, sa, disposition, flags, tmpl);
}

BOOL WINAPI DllMain(HINSTANCE self, DWORD reason, LPVOID reserved)
{
    (void)reserved;
    if (reason == DLL_PROCESS_ATTACH) {
        HMODULE k32;
        void *targetW;
        void *targetA;
        MH_STATUS s1, s2, s3, s4;
        WCHAR msg[320];

        DisableThreadLibraryCalls(self);
        read_env();
        g_main = GetModuleHandleW(NULL);

        s1 = MH_Initialize();
        k32 = GetModuleHandleW(L"kernel32.dll");
        targetW = (void *)GetProcAddress(k32, "CreateFileW");
        targetA = (void *)GetProcAddress(k32, "CreateFileA");
        s2 = s3 = s4 = MH_ERROR_NOT_INITIALIZED;
        if (s1 == MH_OK) {
            s2 = MH_CreateHook(targetW, (void *)HookCreateFileW,
                               (void **)&real_CreateFileW);
            s3 = MH_CreateHook(targetA, (void *)HookCreateFileA,
                               (void **)&real_CreateFileA);
            if (s2 == MH_OK && s3 == MH_OK)
                s4 = MH_EnableHook(MH_ALL_HOOKS);
        }
        swprintf(msg, 320, L"hook installed, init=%d w=%d a=%d enable=%d",
                 s1, s2, s3, s4);
        log_event(msg);
    }
    return TRUE;
}