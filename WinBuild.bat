@echo off
setlocal enabledelayedexpansion

rem ====== read git info ======
set BRANCH=
set COMMIT=
for /f "delims=" %%i in ('git rev-parse --abbrev-ref HEAD') do set BRANCH=%%i
for /f "delims=" %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

if "%BRANCH%"=="" set BRANCH=unknown
if "%COMMIT%"=="" set COMMIT=unknown

echo Branch: %BRANCH%
echo Commit: %COMMIT%

rem ====== go build with ldflags ======
go build -ldflags "-X main.gitBranch=%BRANCH% -X main.gitCommit=%COMMIT%" -o gmk.exe .

if errorlevel 1 (
    echo Build failed.
    exit /b 1
)

echo Build ok: gmk %BRANCH%@%COMMIT%