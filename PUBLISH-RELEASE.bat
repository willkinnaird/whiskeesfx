@echo off
rem Publishes a new WhiskeesFX version: everyone's app picks it up automatically.
rem Needs Git for Windows (https://git-scm.com) and this folder cloned from GitHub.
rem Usage: PUBLISH-RELEASE.bat 1.2.0 "What changed"
setlocal
if "%~1"=="" (echo Usage: PUBLISH-RELEASE.bat 1.2.0 "What changed" & exit /b 1)
set VER=v%~1
set NOTES=%~2
if "%NOTES%"=="" set NOTES=WhiskeesFX %VER%
cd /d "%~dp0"
set COMM=%APPDATA%\Microsoft Flight Simulator 2024\Packages\Community
for %%P in (whiskeesfx-visualfx whiskeesfx-visualfx-effects) do (
  if exist "%COMM%\%%P" (
    echo Copying %%P from Community...
    robocopy "%COMM%\%%P" "packages\%%P" /MIR /NFL /NDL /NJH /NJS >nul
  ) else (
    echo %%P is not in Community: this release won't update it.
  )
)
git add -A || goto :fail
git commit -m "%VER%: %NOTES%"
git tag -a %VER% -m "%NOTES%" || goto :fail
git push && git push origin %VER% || goto :fail
echo.
echo Pushed %VER%. GitHub builds the release in about a minute; installed apps update on their next check.
exit /b 0
:fail
echo Something went wrong (see above).
exit /b 1
