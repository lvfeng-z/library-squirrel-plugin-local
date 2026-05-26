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

$zipPath = "$distDir/local-plugin.zip"
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path "$distDir/*" -DestinationPath $zipPath -Force

Write-Host "Build succeeded! Output in $distDir/" -ForegroundColor Green
