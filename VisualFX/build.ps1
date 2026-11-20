# WhiskeesFX VisualFX build kit: compiles the module with the MSFS 2024 SDK's own compiler and installs it into Community\whiskeesfx-visualfx.
$ErrorActionPreference = 'Stop'
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$sdk  = 'C:\MSFS 2024 SDK\WASM'
$bin  = Join-Path $sdk 'llvm\bin'
$sys  = Join-Path $sdk 'wasi-sysroot'
$lib  = Join-Path $sys 'lib\wasm32-wasi'
$community = Join-Path $env:APPDATA 'Microsoft Flight Simulator 2024\Packages\Community'
$out  = Join-Path $here 'build'
New-Item -ItemType Directory -Force $out | Out-Null
$log = Join-Path $here 'build-log.txt'
"Build started $(Get-Date)" | Out-File $log

function Build($src, $wasm, $defines) {
  $obj = "$wasm.o"
  $cflags = @('--driver-mode=g++', '--target=wasm32-unknown-wasi', "--sysroot=$sys", '-O2',
    '-fno-exceptions', '-fms-extensions', '-fvisibility=hidden', '-mthread-model', 'single',
    '-D_MSFS_WASM=1', '-D__wasi__', '-D_LIBCPP_HAS_NO_THREADS', '-D_WINDLL', '-D_MBCS',
    '-Wno-unused-command-line-argument', '-Wno-ignored-attributes', '-Wno-macro-redefined', '-Wno-pragma-pack',
    "-I$sdk\include", '-c', $src, '-o', $obj) + $defines
  "clang $($cflags -join ' ')" | Out-File $log -Append
  & "$bin\clang-cl.exe" @cflags 2>&1 | Out-File $log -Append
  if ($LASTEXITCODE -ne 0) { throw "Compiling $src failed (see build-log.txt)" }
  $lflags = @($obj, '--whole-archive', "$sdk\WasmVersions\MSFS_WasmVersions.a", '--no-whole-archive',
    "-L$lib", '-lc', "$lib\libclang_rt.builtins-wasm32.a",
    '--no-entry', '--allow-undefined', '--export-dynamic', '--export=__wasm_call_ctors',
    '--export=malloc', '--export=free', '--export=mallinfo', '--export=mchunkit_begin', '--export=mchunkit_next',
    '--export=get_pages_state', '--export=mark_decommit_pages', '--export-table', '--gc-sections', '-O2', '-o', $wasm)
  "wasm-ld $($lflags -join ' ')" | Out-File $log -Append
  & "$bin\wasm-ld.exe" @lflags 2>&1 | Out-File $log -Append
  if ($LASTEXITCODE -ne 0) { throw "Linking $wasm failed (see build-log.txt)" }
  Remove-Item $obj -ErrorAction SilentlyContinue
}

function Install($pkg, $wasm, $name) {
  $dir = Join-Path $community $pkg
  if (-not (Test-Path $dir)) {
    # First install: create the package with its manifest.
    New-Item -ItemType Directory -Force (Join-Path $dir 'modules') | Out-Null
    $manifest = [ordered]@{ dependencies = @(); content_type = 'MISC'; title = 'WhiskeesFX VisualFX'; manufacturer = ''; creator = 'Whiskee';
      package_version = '1.0.0'; minimum_game_version = '1.9.24'; export_type = 'Community'; builder = 'Microsoft Flight Simulator 2024';
      release_notes = @{ neutral = @{ LastUpdate = ''; OlderHistory = '' } } }
    $manifest | ConvertTo-Json -Depth 4 | Out-File (Join-Path $dir 'manifest.json') -Encoding ascii
    "Created package $pkg" | Out-File $log -Append
  }
  Copy-Item $wasm (Join-Path $dir "modules\$name") -Force
  # Rebuild layout.json so MSFS sees the new file size/date.
  $root = (Get-Item $dir).FullName
  $items = Get-ChildItem $root -Recurse -File | Where-Object { $_.Name -notin @('layout.json','manifest.json') } | ForEach-Object {
    [ordered]@{ path = $_.FullName.Substring($root.Length + 1).Replace('\','/'); size = $_.Length; date = $_.LastWriteTimeUtc.ToFileTimeUtc() }
  }
  @{ content = @($items) } | ConvertTo-Json -Depth 4 | Out-File (Join-Path $dir 'layout.json') -Encoding ascii
  "Installed $name into $pkg" | Out-File $log -Append
}

try {
  Build "$here\src\visualfx.cpp" "$out\visualfx.wasm" @()
  Install 'whiskeesfx-visualfx' "$out\visualfx.wasm" 'visualfx.wasm'
  Write-Host ''
  Write-Host 'Done. WhiskeesFX VisualFX was built with the MSFS SDK compiler and installed.' -ForegroundColor Green
  'SUCCESS' | Out-File $log -Append
} catch {
  Write-Host ''
  Write-Host "Build failed: $_" -ForegroundColor Red
  "FAILED: $_" | Out-File $log -Append
}
Write-Host 'A log was saved to build-log.txt.'
Read-Host 'Press Enter to close'
