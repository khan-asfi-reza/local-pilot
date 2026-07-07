@echo off
rem local-pilot entry point (Windows).
rem   pilot start | pilot add <model> | pilot code [--dir X] | pilot run --dir X --task "..."
cd /d "%~dp0"
go build -o bin\pilot.exe .\cmd\pilot || exit /b 1
bin\pilot.exe %*
