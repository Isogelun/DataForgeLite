$ErrorActionPreference = "Stop"

$llvmPath = "C:\Users\a1584\AppData\Local\Microsoft\WinGet\Packages\MartinStorsjo.LLVM-MinGW.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\llvm-mingw-20260311-ucrt-x86_64\bin"
$env:PATH = "$env:PATH;$llvmPath"
$env:CGO_ENABLED = "1"

$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $projectRoot

Write-Host "Building DataForgeLite.exe ..."
Write-Host "Dir: $projectRoot"

& go build -o DataForgeLite.exe .\cmd\
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: build failed, exit code $LASTEXITCODE"
    Read-Host "Press Enter to exit"
    exit $LASTEXITCODE
}
Write-Host "OK: build succeeded"

$outDir = Join-Path $projectRoot "DataForgeLiteClient\DataForgeLiteClient\bin\Debug"
if (-not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}
Copy-Item -Path "DataForgeLite.exe" -Destination $outDir -Force
Write-Host "OK: copied to $outDir"

Read-Host "Press Enter to exit"
