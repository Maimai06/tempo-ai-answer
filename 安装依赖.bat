@echo off
chcp 65001 >nul
title Tempo AI Answer - Install
cd /d "%~dp0"
set "PYTHONIOENCODING=utf-8"

echo ============================================
echo   Installing dependencies (run once)
echo ============================================
echo.

set "BASE="
where python >nul 2>nul && set "BASE=python"

if not defined BASE goto NOPY

if not exist ".venv\Scripts\python.exe" (
  echo Creating virtual environment .venv ...
  "%BASE%" -m venv .venv
)

if not exist ".venv\Scripts\python.exe" goto VENVFAIL

echo Upgrading pip ...
".venv\Scripts\python.exe" -m pip install --upgrade pip --quiet

echo Installing dependencies ...
".venv\Scripts\python.exe" -m pip install -r requirements.txt

echo.
echo Done. Next: run login.bat (first time only).
echo.
pause
exit /b 0

:NOPY
echo.
echo [ERROR] Python not found. Install Python 3.10+ and add it to PATH.
echo.
pause
exit /b 1

:VENVFAIL
echo.
echo [ERROR] Failed to create virtual environment.
echo.
pause
exit /b 1
