@echo off
chcp 65001 >nul
title Tempo AI Answer - Login
cd /d "%~dp0"
set "PYTHONIOENCODING=utf-8"

set "PY="
if exist "%~dp0.venv\Scripts\python.exe" set "PY=%~dp0.venv\Scripts\python.exe"
if not defined PY (
  where python >nul 2>nul && set "PY=python"
)

if not defined PY goto NOPY

"%PY%" main.py login
echo.
pause
exit /b 0

:NOPY
echo.
echo [ERROR] Python not found. Please run install.bat first.
echo.
pause
exit /b 1
