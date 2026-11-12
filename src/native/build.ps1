# Rebuild gate.dll / hook.dll (development only)
# Requires MinGW-w64 (gcc on PATH or in D:\env\MinGW-w64)
$ErrorActionPreference = 'Stop'

$gcc = (Get-Command gcc -ErrorAction SilentlyContinue).Source
if (-not $gcc) { $gcc = 'D:\env\MinGW-w64\bin\gcc.exe' }
if (-not (Test-Path $gcc)) { throw 'Cannot find gcc; install MinGW-w64 or add it to PATH' }
$here = $PSScriptRoot
$assets = Join-Path (Split-Path $here -Parent) 'assets'

$mhDir = Join-Path $here 'minhook\src'
$mh = @(
    (Join-Path $mhDir 'hook.c'),
    (Join-Path $mhDir 'buffer.c'),
    (Join-Path $mhDir 'trampoline.c'),
    (Join-Path $mhDir 'hde\hde64.c'),
    (Join-Path $mhDir 'hde\hde32.c')
)
$inc = @(
    '-I', (Join-Path $here 'minhook\include'),
    '-I', $mhDir,
    '-I', (Join-Path $mhDir 'hde')
)
$common = @('-shared', '-O2', '-s', '-static-libgcc') + $inc

& $gcc @common -o (Join-Path $assets 'gate.dll') (Join-Path $here 'gate.c') @mh
& $gcc @common -o (Join-Path $assets 'hook.dll') (Join-Path $here 'hook.c') @mh

Get-Item (Join-Path $assets 'gate.dll'), (Join-Path $assets 'hook.dll') |
    Select-Object Name, Length | Format-Table -AutoSize
