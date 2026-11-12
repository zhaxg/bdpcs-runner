# Build: native DLLs -> icon resource -> main executable
# Output: dist\BaiduNetdiskRunner.exe and dist\BaiduNetdiskRunner.js
# Requirements: Go and MinGW-w64 (gcc/windres on PATH or in D:\env\MinGW-w64)
param(
    [switch]$SkipNative
)

$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
$src = Join-Path $root 'src'
$dist = Join-Path $root 'dist'
$mingwBin = 'D:\env\MinGW-w64\bin'

function Find-Tool([string]$Name) {
    $tool = Get-Command $Name -ErrorAction SilentlyContinue
    if ($tool) {
        return $tool.Source
    }

    $fallback = Join-Path $mingwBin "$Name.exe"
    if (Test-Path $fallback) {
        return $fallback
    }

    throw "Cannot find $Name; install MinGW-w64 or add it to PATH"
}

$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) {
    throw 'Cannot find go; install Go or add it to PATH'
}

New-Item -ItemType Directory -Force -Path $dist | Out-Null

if (-not $SkipNative) {
    & (Join-Path $src 'native\build.ps1')
    if ($LASTEXITCODE -ne 0) {
        throw 'Native DLL build failed'
    }
}

$windres = Find-Tool 'windres'
Push-Location $src
try {
    & $windres -O coff -o runner.syso runner.rc
    if ($LASTEXITCODE -ne 0) {
        throw 'Icon resource build failed'
    }

    & $go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $dist 'BaiduNetdiskRunner.exe') .
    if ($LASTEXITCODE -ne 0) {
        throw 'Main executable build failed'
    }
}
finally {
    Pop-Location
}

Get-Item (Join-Path $dist 'BaiduNetdiskRunner.exe') |
    Select-Object FullName, Length, LastWriteTime | Format-List

$runnerJS = Join-Path $src 'assets\BaiduNetdiskRunner.js'
$distJS = Join-Path $dist 'BaiduNetdiskRunner.js'
Copy-Item -LiteralPath $runnerJS -Destination $distJS -Force
Get-Item $distJS | Select-Object FullName, Length, LastWriteTime | Format-List
