$ErrorActionPreference = "Stop"

$projectName = "local_import"
$distDir = "dist"

Write-Host "Building $projectName plugin..."

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

$zipPath = "$distDir/local-plugin.zip"
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path "$distDir/*" -DestinationPath $zipPath -Force

Write-Host "Build succeeded! Output in $distDir/" -ForegroundColor Green
