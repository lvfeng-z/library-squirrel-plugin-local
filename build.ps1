$ErrorActionPreference = "Stop"

$projectName = "local_import"
$distDir = "dist"
$frontendDir = "frontend"

Write-Host "Building frontend..." -ForegroundColor Cyan

Push-Location $frontendDir
if (-not (Test-Path "node_modules")) {
    yarn install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Frontend install failed!" -ForegroundColor Red
        Pop-Location
        exit 1
    }
}
yarn build
if ($LASTEXITCODE -ne 0) {
    Write-Host "Frontend build failed!" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

Write-Host "Building $projectName plugin..." -ForegroundColor Cyan

go build -o "$projectName.exe" .

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# 创建 dist 目录
if (Test-Path $distDir) { Remove-Item -Recurse -Force $distDir }
New-Item -ItemType Directory -Path $distDir | Out-Null

# 复制文件到 dist
Copy-Item "plugin.json" "$distDir/"
Move-Item "$projectName.exe" "$distDir/"
if (Test-Path "views") {
    Copy-Item -Recurse "views" "$distDir/"
}

# Build identity stamping: inject a build id into dist plugin.json as "buildId".
# The main app compares buildId to detect same-build installs. Invariants:
#   - clean tree  -> pure "git describe" (release semantics unchanged);
#   - dirty tree  -> "<describe>-<digest8>", where digest8 is the first 8 hex chars
#     of a SHA-256 over the dist payload (exe + views + unstamped manifest), so
#     different shipped content can never share one buildId.
# Why hash the payload instead of git state: "git stash create" drops untracked
# files, and views/*.js build artifacts are gitignored -- git state cannot see the
# bytes that actually ship. Payload hashing covers them all; exe rebuild drift
# (toolchain nondeterminism) only changes the digest and costs one extra reinstall.
# Dirtiness is detected via "git status --porcelain" (untracked files included),
# which is broader than describe's -dirty marker (tracked modifications only):
# when only untracked files differ, "-dirty" is appended before the digest.
# Keep this block ASCII-only: powershell -File decodes no-BOM scripts as GBK, and CJK comments can swallow the next line.
# Text insertion instead of ConvertTo-Json round-trip (avoids PS5.1 depth/escape mangling); write UTF-8 without BOM (Go json rejects BOM).
$buildId = git describe --tags --always --dirty
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($buildId)) {
    Write-Host "Build failed! git describe failed (not a git repo?), buildId is required." -ForegroundColor Red
    exit 1
}
$buildId = $buildId.Trim()
$dirty = -not [string]::IsNullOrWhiteSpace((git status --porcelain))
# .NET-based file hash: Get-FileHash is a cmdlet and proved unavailable in the
# "powershell -NoProfile -File" child used by the main repo build pipeline
# (module auto-loading may be broken there); zero-cmdlet keeps it bulletproof.
function Get-FileSha256([string]$path) {
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $stream = [System.IO.File]::OpenRead($path)
        try { $bytes = $sha.ComputeHash($stream) } finally { $stream.Dispose() }
    } finally { $sha.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
}
if ($dirty) {
    $distRoot = (Resolve-Path $distDir).Path
    $payloadLines = foreach ($f in (Get-ChildItem -LiteralPath $distDir -Recurse -File | Sort-Object FullName)) {
        $rel = ($f.FullName.Substring($distRoot.Length + 1)) -replace '\\', '/'
        $hash = Get-FileSha256 $f.FullName
        "$rel $hash"
    }
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $digestBytes = $sha.ComputeHash([System.Text.Encoding]::UTF8.GetBytes(($payloadLines -join "`n")))
    } finally {
        $sha.Dispose()
    }
    $digest8 = ([BitConverter]::ToString($digestBytes)).Replace('-', '').ToLowerInvariant().Substring(0, 8)
    if (-not $buildId.EndsWith('-dirty')) { $buildId = "$buildId-dirty" }
    $buildId = "$buildId-$digest8"
}
$manifestPath = Join-Path $distDir 'plugin.json'
$manifestText = [System.IO.File]::ReadAllText($manifestPath)
$stamped = $manifestText -replace '^\s*\{', ('{"buildId": "' + $buildId + '",')
[System.IO.File]::WriteAllText($manifestPath, $stamped, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "  buildId: $buildId"

$zipPath = "$distDir/local-plugin.zip"
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path "$distDir/*" -DestinationPath $zipPath -Force

Write-Host "Build succeeded! Output in $distDir/" -ForegroundColor Green
