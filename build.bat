@echo off
setlocal

set LLVM_PATH=C:\Users\a1584\AppData\Local\Microsoft\WinGet\Packages\MartinStorsjo.LLVM-MinGW.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\llvm-mingw-20260311-ucrt-x86_64\bin
set PATH=%PATH%;%LLVM_PATH%
set CGO_ENABLED=1

echo ==========================================
echo  编译 Go 后端 DataForgeLite.exe
echo ==========================================

cd /d %~dp0
echo 当前目录: %CD%
echo GCC 版本:
gcc --version

go build -o DataForgeLite.exe .\cmd\
if %ERRORLEVEL% neq 0 (
    echo [错误] Go 编译失败，退出码: %ERRORLEVEL%
    pause
    exit /b %ERRORLEVEL%
)
echo [OK] DataForgeLite.exe 编译成功

echo.
echo ==========================================
echo  复制到 WPF 输出目录
echo ==========================================

set OUT_DIR=%~dp0DataForgeLiteClient\DataForgeLiteClient\bin\Debug
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"
copy /Y DataForgeLite.exe "%OUT_DIR%\DataForgeLite.exe"
if %ERRORLEVEL% neq 0 (
    echo [错误] 复制失败
    pause
    exit /b %ERRORLEVEL%
)
echo [OK] 已复制到 %OUT_DIR%

echo.
echo 完成！
pause
endlocal
